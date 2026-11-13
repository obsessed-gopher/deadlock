// In-memory imitation of the Go REST gateway over captured fixtures
// (mock/capture.ts). Same routes, JSON shape and error format as the real
// server, so pages and e2e tests cannot tell the difference. Kept free of
// HTTP so it is unit-tested directly; mock/server.ts adds the listener.
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

import type {
  GameData,
  HeroStats,
  ListPlayerHeroStatsResponse,
  ListPlayerMatchFactsResponse,
  ListPlayerMatchesResponse,
  ListPlayerPeersResponse,
  ListProfilesResponse,
  ListSeenMatchesResponse,
  Match,
  Player,
  PlayerMatch,
  PlayerMatchRow,
  Profile,
  ResolveSteamVanityResponse,
  RpcStatus,
  SeenMatch,
  SyncPlayerResponse,
} from "../src/api/schema.gen.ts";

import { type QueueKey, apiQueue, queueOf } from "../src/lib/queue.ts";

export interface Fixtures {
  gameData: GameData;
  accountId: number;
  rows: PlayerMatchRow[];
  /** Facts of the same matches, oldest first, as the real server has them. */
  facts: ListPlayerMatchFactsResponse;
  /** Server-side aggregate over the whole history, not just these rows. */
  peers: ListPlayerPeersResponse;
  /** Keyed by `${matchId}-${accountId}`. */
  playerMatches: Map<string, PlayerMatch>;
}

export function loadFixtures(dir: string): Fixtures {
  const read = <T>(...p: string[]) => JSON.parse(readFileSync(join(dir, ...p), "utf8")) as T;
  const { accountId, matches } = read<{ accountId: number; matches: PlayerMatchRow[] }>("matches.json");
  const playerMatches = new Map<string, PlayerMatch>();
  for (const f of readdirSync(join(dir, "player-matches"))) {
    if (f.endsWith(".json")) playerMatches.set(f.slice(0, -5), read<PlayerMatch>("player-matches", f));
  }
  return {
    gameData: read<GameData>("gamedata.json"),
    accountId,
    rows: matches,
    facts: read<ListPlayerMatchFactsResponse>("facts.json"),
    peers: read<ListPlayerPeersResponse>("peers.json"),
    playerMatches,
  };
}

export interface Response {
  status: number;
  body: unknown;
}

/** Ids that make the mock fail on purpose (see handle). */
export const ERROR_MATCH = 500;
export const UNAVAILABLE_PLAYER = 503;
/** Unknown player (404) whose sync fails with Unavailable. */
export const SYNC_FAILS_PLAYER = 504;
/** Steam custom URL names: one resolves to the fixture account, one fails like an unreachable Steam. */
export const KNOWN_VANITY = "mock-player";
export const DOWN_VANITY = "steam-down";

/** Name of the fixture account in the mock's synthetic Steam profiles. */
export const OWNER_NAME = "Mock Owner";
const NAME_WORDS = ["kestrel", "basalt", "nimbus", "quill", "ember", "tundra", "cobalt", "sable", "fjord", "lumen"];

/**
 * Synthetic Steam profile: real names and avatars of strangers must not end
 * up in fixtures of a public repository. Every 7th account has none (not
 * fetched yet / no profile), as happens with the real API.
 */
export function mockProfile(accountId: number, ownerId: number): Profile | null {
  if (accountId !== ownerId && accountId % 7 === 0) return null;
  const name = accountId === ownerId ? OWNER_NAME : `${NAME_WORDS[accountId % NAME_WORDS.length]}_${accountId % 1000}`;
  return {
    accountId,
    personaName: name,
    profileUrl: `https://steamcommunity.com/profiles/${BigInt(accountId) + STEAM_ID64_BASE}/`,
    avatar: "",
    avatarMedium: "",
    avatarFull: "",
    country: "",
  };
}

const STEAM_ID64_BASE = 76561197960265728n;
const MAX_U32 = 0xffffffffn;

/** Same rule as source.NormalizeAccountID in Go: account id or SteamID64. */
export function normalizeAccountId(s: string): number | null {
  if (!/^\d{1,20}$/.test(s)) return null;
  const v = BigInt(s);
  if (v === 0n) return null;
  if (v <= MAX_U32) return Number(v);
  if (v > STEAM_ID64_BASE && v - STEAM_ID64_BASE <= MAX_U32) return Number(v - STEAM_ID64_BASE);
  return null;
}

function error(status: number, code: number, message: string): Response {
  const body: RpcStatus = { code, message, details: [] };
  return { status, body };
}

const notFound = () => error(404, 5, "not found");
const badRequest = (msg: string) => error(400, 3, msg);
const ok = (body: unknown): Response => ({ status: 200, body });

// ListPlayerMatches sort keys -> the row value they sort by (as in Go,
// db.MatchSortValue).
const SORT_VALUES: Record<string, (r: PlayerMatchRow) => string | number> = {
  MATCH_SORT_UNSPECIFIED: (r) => r.startTime,
  MATCH_SORT_START_TIME: (r) => r.startTime,
  MATCH_SORT_DURATION: (r) => r.durationS,
  MATCH_SORT_KILLS: (r) => r.kills,
  MATCH_SORT_DEATHS: (r) => r.deaths,
  MATCH_SORT_ASSISTS: (r) => r.assists,
  MATCH_SORT_NET_WORTH: (r) => r.netWorth,
  MATCH_SORT_LAST_HITS: (r) => r.lastHits,
  MATCH_SORT_HERO_LEVEL: (r) => r.heroLevel,
};

const RESULTS: Record<string, boolean | undefined> = {
  MATCH_RESULT_UNSPECIFIED: undefined,
  MATCH_RESULT_WIN: true,
  MATCH_RESULT_LOSS: false,
};

const META_STATUSES = [
  "META_STATUS_UNSPECIFIED",
  "META_STATUS_PENDING",
  "META_STATUS_OK",
  "META_STATUS_EXPIRED",
  "META_STATUS_FAILED",
];

const byMatchDesc = (a: PlayerMatchRow, b: PlayerMatchRow) => {
  const x = BigInt(a.matchId);
  const y = BigInt(b.matchId);
  return x === y ? 0 : x > y ? -1 : 1;
};

export function createMockApi(fx: Fixtures, now: () => number = () => Date.now()) {
  const fullMatches = new Set([...fx.playerMatches.keys()].map((k) => k.split("-")[0]));
  // Only matches with a captured breakdown count as having metadata, so every
  // "OK" row links to a page the mock can serve.
  const rows = fx.rows
    .map((r) => ({
      ...r,
      metaStatus: fullMatches.has(r.matchId)
        ? ("META_STATUS_OK" as const)
        : r.metaStatus === "META_STATUS_OK"
          ? ("META_STATUS_PENDING" as const)
          : r.metaStatus,
    }))
    .sort(byMatchDesc);
  // Deterministic "last sync": an hour after the newest match.
  let lastSyncedAt = Math.max(0, ...rows.map((r) => Number(r.startTime))) + 3600;

  function player(): Player {
    return {
      accountId: fx.accountId,
      syncState: "SYNC_STATE_IDLE",
      syncError: "",
      lastSyncedAt: String(lastSyncedAt),
      matchesTotal: rows.length,
      matchesWithMeta: rows.filter((r) => r.metaStatus === "META_STATUS_OK").length,
      matchesPending: rows.filter((r) => r.metaStatus === "META_STATUS_PENDING").length,
      wins: rows.filter((r) => r.won).length,
    };
  }

  function heroStats(): HeroStats[] {
    const by = new Map<number, HeroStats>();
    for (const r of rows) {
      const h = by.get(r.heroId) ?? {
        heroId: r.heroId,
        matches: 0,
        wins: 0,
        kills: 0,
        deaths: 0,
        assists: 0,
        netWorth: 0,
        lastHits: 0,
        durationS: 0,
        lastPlayed: "0",
      };
      h.matches++;
      h.wins += r.won ? 1 : 0;
      h.kills += r.kills;
      h.deaths += r.deaths;
      h.assists += r.assists;
      h.netWorth += r.netWorth;
      h.lastHits += r.lastHits;
      h.durationS += r.durationS;
      if (Number(r.startTime) > Number(h.lastPlayed)) h.lastPlayed = r.startTime;
      by.set(r.heroId, h);
    }
    return [...by.values()].sort((a, b) => b.matches - a.matches || a.heroId - b.heroId);
  }

  function listMatches(q: URLSearchParams): Response {
    const get = (camel: string, snake: string) => q.get(camel) ?? q.get(snake) ?? "";
    const sizeRaw = Number(get("pageSize", "page_size") || 0);
    const size = sizeRaw <= 0 ? 50 : Math.min(sizeRaw, 200);

    const sortName = get("sort", "sort") || "MATCH_SORT_UNSPECIFIED";
    const value = SORT_VALUES[sortName];
    if (!value) return badRequest("unknown sort");
    const result = get("result", "result") || "MATCH_RESULT_UNSPECIFIED";
    if (!(result in RESULTS)) return badRequest("unknown result");
    const meta = get("metaStatus", "meta_status") || "META_STATUS_UNSPECIFIED";
    if (!META_STATUSES.includes(meta)) return badRequest("unknown meta_status");
    const asc = get("ascending", "ascending") === "true";
    const heroId = Number(get("heroId", "hero_id") || 0);
    const gameMode = Number(get("gameMode", "game_mode") || 0);
    const queue = get("queue", "queue") || "MATCH_QUEUE_UNSPECIFIED";
    const queueKey = (Object.keys(apiQueue) as QueueKey[]).find((k) => apiQueue[k] === queue);
    if (queue !== "MATCH_QUEUE_UNSPECIFIED" && !queueKey) return badRequest("unknown queue");

    // Same token format as the Go server: "<sort value>_<match id>".
    const token = get("pageToken", "page_token");
    let after: { v: bigint; id: bigint } | null = null;
    if (token) {
      const m = /^(-?\d+)_(\d+)$/.exec(token);
      if (!m) return badRequest("bad page_token");
      after = { v: BigInt(m[1]!), id: BigInt(m[2]!) };
    }

    const filtered = rows.filter(
      (r) =>
        (!heroId || r.heroId === heroId) &&
        (RESULTS[result] === undefined || r.won === RESULTS[result]) &&
        (!gameMode || r.gameMode === gameMode) &&
        (!queueKey || queueOf(r.gameMode, r.matchMode) === queueKey) &&
        (meta === "META_STATUS_UNSPECIFIED" || r.metaStatus === meta),
    );
    const key = (r: PlayerMatchRow) => ({ v: BigInt(value(r)), id: BigInt(r.matchId) });
    const cmp = (a: { v: bigint; id: bigint }, b: { v: bigint; id: bigint }) =>
      a.v === b.v ? (a.id === b.id ? 0 : a.id < b.id ? -1 : 1) : a.v < b.v ? -1 : 1;
    const sign = asc ? 1 : -1;
    const page = filtered
      .sort((a, b) => sign * cmp(key(a), key(b)))
      .filter((r) => !after || sign * cmp(key(r), after) > 0)
      .slice(0, size);
    const last = page.at(-1);
    const body: ListPlayerMatchesResponse = {
      matches: page,
      nextPageToken: page.length === size && last ? `${value(last)}_${last.matchId}` : "",
      total: filtered.length,
    };
    return ok(body);
  }

  function match(id: string): Response {
    for (const [key, pm] of fx.playerMatches) {
      if (key.startsWith(`${id}-`) && pm.match) return ok(pm.match);
    }
    const r = rows.find((x) => x.matchId === id);
    if (!r) return notFound();
    // Known from history only: header without a scoreboard, like the real API.
    const header: Match = {
      matchId: r.matchId,
      startTime: r.startTime,
      durationS: r.durationS,
      gameMode: r.gameMode,
      matchMode: r.matchMode,
      winningTeam: r.won ? r.team : 1 - r.team,
      metaStatus: r.metaStatus,
      players: [],
    };
    return ok(header);
  }

  /**
   * Seen matches come from the full breakdowns only: the fixtures have
   * rosters for just those few matches, for every player in them.
   */
  function seenMatches(accountId: number): SeenMatch[] {
    const matches = new Map<string, Match>();
    for (const pm of fx.playerMatches.values()) if (pm.match) matches.set(pm.match.matchId, pm.match);
    const out: SeenMatch[] = [];
    for (const m of matches.values()) {
      const p = m.players.find((x) => x.accountId === accountId);
      if (!p) continue;
      out.push({
        matchId: m.matchId,
        startTime: m.startTime,
        durationS: m.durationS,
        gameMode: m.gameMode,
        matchMode: m.matchMode,
        heroId: p.heroId,
        team: p.team,
        won: p.team === m.winningTeam,
        kills: p.kills,
        deaths: p.deaths,
        assists: p.assists,
        netWorth: p.netWorth,
        lastHits: p.lastHits,
        playerDamage: p.playerDamage,
        playerHealing: p.playerHealing,
        level: p.level,
      });
    }
    return out.sort((a, b) => Number(BigInt(a.startTime) - BigInt(b.startTime)));
  }

  /** Handles one request; path may include a query string. */
  function handle(method: string, url: string): Response {
    const u = new URL(url, "http://mock");
    const parts = u.pathname.split("/").filter(Boolean);
    if (u.pathname === "/healthz") return ok("ok");
    if (parts[0] !== "v1") return notFound();

    // Error triggers for e2e tests of the error pages: match 500 fails with
    // Internal, player 503 with Unavailable, the sync of player 504 with
    // Unavailable, like the real gateway.
    if (parts[1] === "matches" && parts[2] === String(ERROR_MATCH)) return error(500, 13, "internal error");
    if (parts[1] === "players" && parts[2]?.split(":")[0] === String(UNAVAILABLE_PLAYER)) {
      return error(503, 14, "provider unavailable");
    }
    if (method === "POST" && parts[1] === "players" && parts[2] === `${SYNC_FAILS_PLAYER}:sync`) {
      return error(503, 14, "sync: provider unavailable");
    }

    if (method === "GET" && parts.length === 2 && parts[1] === "gamedata") return ok(fx.gameData);

    if (method === "GET" && parts.length === 2 && parts[1] === "profiles") {
      const raw = u.searchParams.getAll("account_ids");
      if (raw.length > 1000) return badRequest(`${raw.length} account_ids, max 1000`);
      const profiles: Profile[] = [];
      for (const r of raw) {
        const id = normalizeAccountId(r);
        if (id === null) return badRequest("account_id is neither an account id nor a SteamID64");
        const p = mockProfile(id, fx.accountId);
        if (p) profiles.push(p);
      }
      const body: ListProfilesResponse = { profiles };
      return ok(body);
    }

    if (method === "GET" && parts.length === 4 && parts[1] === "steam" && parts[2] === "vanity") {
      const name = decodeURIComponent(parts[3] ?? "");
      if (!/^[A-Za-z0-9_-]{2,64}$/.test(name)) return badRequest("not a steam custom url name");
      if (name === DOWN_VANITY) return error(503, 14, "steam: connection refused");
      if (name !== KNOWN_VANITY) return notFound();
      const body: ResolveSteamVanityResponse = { accountId: fx.accountId };
      return ok(body);
    }

    if (parts[1] === "players" && parts[2]) {
      const [rawId, verb] = parts[2].split(":");
      const id = normalizeAccountId(rawId ?? "");
      if (id === null) return badRequest("account_id is neither an account id nor a SteamID64");
      const tracked = id === fx.accountId;
      if (method === "POST" && verb === "sync" && parts.length === 3) {
        if (tracked) lastSyncedAt = Math.floor(now() / 1000);
        const body: SyncPlayerResponse = { newMatches: 0, queuedMeta: tracked ? player().matchesPending : 0 };
        return ok(body);
      }
      if (method !== "GET" || verb) return notFound();
      if (parts.length === 3) return tracked ? ok(player()) : notFound();
      if (parts.length === 4 && parts[3] === "matches") {
        return tracked ? listMatches(u.searchParams) : ok({ matches: [], nextPageToken: "" });
      }
      if (parts.length === 4 && parts[3] === "facts") {
        const body: ListPlayerMatchFactsResponse = tracked
          ? fx.facts
          : { matches: [], soulMarksS: fx.facts.soulMarksS, soulTargets: fx.facts.soulTargets };
        return ok(body);
      }
      if (parts.length === 4 && parts[3] === "seen") {
        const body: ListSeenMatchesResponse = { matches: seenMatches(id) };
        return ok(body);
      }
      if (parts.length === 4 && parts[3] === "peers") {
        const body: ListPlayerPeersResponse = tracked
          ? fx.peers
          : {
              ...fx.peers,
              matches: 0,
              wins: 0,
              allies: [],
              enemies: [],
              heroes: [],
              partySizes: [],
            };
        return ok(body);
      }
      if (parts.length === 4 && parts[3] === "heroes") {
        const body: ListPlayerHeroStatsResponse = { heroes: tracked ? heroStats() : [] };
        return ok(body);
      }
      return notFound();
    }

    if (method === "GET" && parts[1] === "matches" && parts[2]) {
      const mid = parts[2];
      if (!/^\d+$/.test(mid) || /^0+$/.test(mid)) return badRequest("match_id is required");
      if (parts.length === 3) return match(mid);
      if (parts.length === 5 && parts[3] === "players") {
        const aid = normalizeAccountId(parts[4] ?? "");
        if (aid === null) return badRequest("account_id is neither an account id nor a SteamID64");
        const pm = fx.playerMatches.get(`${mid}-${aid}`);
        return pm ? ok(pm) : notFound();
      }
    }
    return notFound();
  }

  return { handle };
}
