// Server-side client for the Go REST gateway. Pages fetch on the server, so
// the API never has to be exposed to the browser and needs no CORS.
import type {
  GameData,
  ListPlayerHeroStatsResponse,
  ListPlayerMatchesResponse,
  Match,
  Player,
  PlayerMatch,
  RpcStatus,
  SyncPlayerResponse,
} from "./schema.gen.ts";

const API_URL = (process.env.API_URL ?? "http://localhost:8080").replace(/\/+$/, "");

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

type FetchOptions = RequestInit & { next?: { revalidate?: number } };

async function call<T>(path: string, init: FetchOptions = { cache: "no-store" }): Promise<T> {
  const res = await fetch(API_URL + path, init);
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`;
    try {
      const body = (await res.json()) as RpcStatus;
      if (body.message) message = body.message;
    } catch {
      // not a gateway error body; keep the HTTP status text
    }
    throw new ApiError(res.status, message);
  }
  return (await res.json()) as T;
}

/** Account and match ids are decimal integers; anything else is a 404. */
export function isId(s: string): boolean {
  return /^\d{1,20}$/.test(s) && !/^0+$/.test(s);
}

export function isNotFound(err: unknown): boolean {
  return err instanceof ApiError && err.status === 404;
}

export interface MatchesQuery {
  pageSize?: number;
  pageToken?: string;
  heroId?: number;
}

export const api = {
  player: (accountId: string) => call<Player>(`/v1/players/${accountId}`),

  matches: (accountId: string, q: MatchesQuery = {}) => {
    const p = new URLSearchParams();
    if (q.pageSize) p.set("pageSize", String(q.pageSize));
    if (q.pageToken) p.set("pageToken", q.pageToken);
    if (q.heroId) p.set("heroId", String(q.heroId));
    const qs = p.size ? `?${p}` : "";
    return call<ListPlayerMatchesResponse>(`/v1/players/${accountId}/matches${qs}`);
  },

  heroStats: (accountId: string) => call<ListPlayerHeroStatsResponse>(`/v1/players/${accountId}/heroes`),

  sync: (accountId: string) =>
    call<SyncPlayerResponse>(`/v1/players/${accountId}:sync`, {
      method: "POST",
      cache: "no-store",
      headers: { "content-type": "application/json" },
      body: "{}",
    }),

  match: (matchId: string) => call<Match>(`/v1/matches/${matchId}`),

  playerMatch: (matchId: string, accountId: string) =>
    call<PlayerMatch>(`/v1/matches/${matchId}/players/${accountId}`),

  // Changes only with game patches.
  gameData: () => call<GameData>("/v1/gamedata", { next: { revalidate: 3600 } }),
};
