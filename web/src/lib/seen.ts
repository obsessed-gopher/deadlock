// A player's stored matches with metadata (ListSeenMatches): summaries for
// profiles of untracked players and the "me vs them" comparison. Pure.
import type { SeenMatch } from "../api/schema.gen.ts";
import { type Bucket, winRate } from "./progress.ts";
import { type QueueKey, queueOf } from "./queue.ts";

export interface Summary extends Bucket {
  kills: number;
  deaths: number;
  assists: number;
  /** (kills + assists) / max(1, deaths) over all matches. */
  kda: number;
  soulsPerMin: number;
  damagePerMin: number;
  healingPerMin: number;
  lastHits: number;
  /** Seconds. */
  duration: number;
}

/** Per-match averages; per-minute rates over the total time played. */
export function summarize(ms: SeenMatch[]): Summary {
  const n = ms.length;
  const sum = (f: (m: SeenMatch) => number) => ms.reduce((acc, m) => acc + f(m), 0);
  const minutes = sum((m) => m.durationS) / 60;
  const per = (v: number) => (n ? v / n : 0);
  const k = sum((m) => m.kills);
  const d = sum((m) => m.deaths);
  const a = sum((m) => m.assists);
  return {
    matches: n,
    wins: ms.filter((m) => m.won).length,
    kills: per(k),
    deaths: per(d),
    assists: per(a),
    kda: n ? (k + a) / Math.max(1, d) : 0,
    soulsPerMin: minutes ? sum((m) => m.netWorth) / minutes : 0,
    damagePerMin: minutes ? sum((m) => m.playerDamage) / minutes : 0,
    healingPerMin: minutes ? sum((m) => m.playerHealing) / minutes : 0,
    lastHits: per(sum((m) => m.lastHits)),
    duration: per(sum((m) => m.durationS)),
  };
}

export interface HeroLine extends Bucket {
  heroId: number;
  kda: number;
  soulsPerMin: number;
}

/** Heroes by matches played, most first (ties: higher win rate, then id). */
export function heroLines(ms: SeenMatch[]): HeroLine[] {
  const by = new Map<number, SeenMatch[]>();
  for (const m of ms) by.set(m.heroId, [...(by.get(m.heroId) ?? []), m]);
  return [...by]
    .map(([heroId, list]) => {
      const s = summarize(list);
      return { heroId, matches: s.matches, wins: s.wins, kda: s.kda, soulsPerMin: s.soulsPerMin };
    })
    .sort((x, y) => y.matches - x.matches || winRate(y) - winRate(x) || x.heroId - y.heroId);
}

export interface SharedMatch {
  matchId: string;
  startTime: string;
  durationS: number;
  queue: QueueKey;
  /** Same team. */
  together: boolean;
  a: SeenMatch;
  b: SeenMatch;
}

export interface Shared {
  /** Newest first. */
  matches: SharedMatch[];
  /** On one team; wins are A's (= B's). */
  together: Bucket;
  /** On opposite teams; wins are A's. */
  against: Bucket;
}

const newestFirst = (p: SharedMatch, q: SharedMatch) => {
  const d = BigInt(q.startTime) - BigInt(p.startTime) || BigInt(q.matchId) - BigInt(p.matchId);
  return d > 0n ? 1 : d < 0n ? -1 : 0;
};

/** Matches both players were in, from A's point of view. */
export function shared(a: SeenMatch[], b: SeenMatch[]): Shared {
  const bs = new Map(b.map((m) => [m.matchId, m]));
  const matches: SharedMatch[] = [];
  for (const x of a) {
    const y = bs.get(x.matchId);
    if (!y) continue;
    matches.push({
      matchId: x.matchId,
      startTime: x.startTime,
      durationS: x.durationS,
      queue: queueOf(x.gameMode, x.matchMode),
      together: x.team === y.team,
      a: x,
      b: y,
    });
  }
  matches.sort(newestFirst);
  const bucket = (list: SharedMatch[]): Bucket => ({ matches: list.length, wins: list.filter((m) => m.a.won).length });
  return {
    matches,
    together: bucket(matches.filter((m) => m.together)),
    against: bucket(matches.filter((m) => !m.together)),
  };
}

/** Matches of one queue; "" keeps all. */
export function inQueue<T extends { gameMode: number; matchMode: number }>(ms: T[], queue: QueueKey | ""): T[] {
  return queue ? ms.filter((m) => queueOf(m.gameMode, m.matchMode) === queue) : ms;
}
