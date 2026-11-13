// Server-side client for the Go REST gateway. Pages fetch on the server, so
// the API never has to be exposed to the browser and needs no CORS.
// Every failure becomes an ApiError with a kind the UI can localize.
import { ApiError, kindOf } from "../lib/errors.ts";
import { type MatchListState, matchApiParams } from "../lib/matchQuery.ts";
import { NO_PROFILES, type Profiles, profileMap, uniqueIds } from "../lib/profiles.ts";
import type {
  GameData,
  ListPlayerHeroStatsResponse,
  ListPlayerMatchFactsResponse,
  ListPlayerPeersResponse,
  ListProfilesResponse,
  ResolveSteamVanityResponse,
  ListPlayerMatchesResponse,
  Match,
  Player,
  PlayerMatch,
  RpcStatus,
  SyncPlayerResponse,
} from "./schema.gen.ts";

const API_URL = (process.env.API_URL ?? "http://localhost:8080").replace(/\/+$/, "");
// A hung API must not hang the page: fail into a localized "unavailable".
const TIMEOUT_MS = 15_000;

type FetchOptions = RequestInit & { next?: { revalidate?: number } };

async function call<T>(path: string, init: FetchOptions = { cache: "no-store" }): Promise<T> {
  let res: Response;
  try {
    res = await fetch(API_URL + path, { signal: AbortSignal.timeout(TIMEOUT_MS), ...init });
  } catch (err) {
    throw new ApiError("unavailable", 0, err instanceof Error ? err.message : String(err));
  }
  if (!res.ok) {
    let body: Partial<RpcStatus> = {};
    try {
      body = (await res.json()) as RpcStatus;
    } catch {
      // not a gateway error body (proxy page, empty response)
    }
    throw new ApiError(kindOf(res.status, body.code), res.status, body.message || res.statusText);
  }
  try {
    return (await res.json()) as T;
  } catch (err) {
    throw new ApiError("internal", res.status, `bad JSON: ${String(err)}`);
  }
}

/** Account and match ids are positive decimal integers. */
export function isId(s: string): boolean {
  return /^\d{1,20}$/.test(s) && !/^0+$/.test(s);
}

export const PAGE_SIZE = 50;

export const api = {
  player: (accountId: string) => call<Player>(`/v1/players/${accountId}`),

  matches: (accountId: string, state: MatchListState) =>
    call<ListPlayerMatchesResponse>(`/v1/players/${accountId}/matches?${matchApiParams(state, PAGE_SIZE)}`),

  heroStats: (accountId: string) => call<ListPlayerHeroStatsResponse>(`/v1/players/${accountId}/heroes`),

  facts: (accountId: string) => call<ListPlayerMatchFactsResponse>(`/v1/players/${accountId}/facts`),

  peers: (accountId: string) => call<ListPlayerPeersResponse>(`/v1/players/${accountId}/peers`),

  sync: (accountId: string) =>
    call<SyncPlayerResponse>(`/v1/players/${accountId}:sync`, {
      method: "POST",
      cache: "no-store",
      headers: { "content-type": "application/json" },
      body: "{}",
    }),

  /**
   * Steam profiles by account id. Names and avatars are decoration: any
   * failure yields no profiles (pages fall back to "Player <id>"), never an
   * error page.
   */
  profiles: async (ids: Iterable<string | number>): Promise<Profiles> => {
    const list = uniqueIds(ids);
    if (list.length === 0) return NO_PROFILES;
    try {
      const res = await call<ListProfilesResponse>(`/v1/profiles?${list.map((id) => `account_ids=${id}`).join("&")}`);
      return profileMap(res.profiles);
    } catch {
      return NO_PROFILES;
    }
  },

  resolveVanity: (name: string) =>
    call<ResolveSteamVanityResponse>(`/v1/steam/vanity/${encodeURIComponent(name)}`, { cache: "no-store" }),

  match: (matchId: string) => call<Match>(`/v1/matches/${matchId}`),

  playerMatch: (matchId: string, accountId: string) =>
    call<PlayerMatch>(`/v1/matches/${matchId}/players/${accountId}`),

  // Changes only with game patches.
  gameData: () => call<GameData>("/v1/gamedata", { next: { revalidate: 3600 } }),
};
