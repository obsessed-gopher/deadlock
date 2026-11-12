// URL state of the match history table: filters, sort and page live in the
// query string, so every view is a link and works without JavaScript.
// Sorting and filtering run on the server (ListPlayerMatches); invalid values
// fall back to defaults instead of producing an error page.
import type { MatchResult, MatchSort, MetaStatus } from "../api/schema.gen.ts";
import type { SortDir } from "./table.ts";

export const MATCH_SORTS = ["date", "duration", "kills", "deaths", "assists", "souls", "lastHits", "level"] as const;
export type MatchSortKey = (typeof MATCH_SORTS)[number];

export const RESULTS = ["", "win", "loss"] as const;
export type ResultFilter = (typeof RESULTS)[number];

export const METAS = ["", "ok", "pending", "expired", "failed"] as const;
export type MetaFilter = (typeof METAS)[number];

export interface MatchListState {
  hero: number;
  result: ResultFilter;
  mode: number;
  meta: MetaFilter;
  sort: MatchSortKey;
  dir: SortDir;
  /** Opaque token of the page; "" = first page. */
  page: string;
  /** Rows before this page, for the "51–100 of N" label only. */
  from: number;
}

export const DEFAULT_STATE: MatchListState = {
  hero: 0,
  result: "",
  mode: 0,
  meta: "",
  sort: "date",
  dir: "desc",
  page: "",
  from: 0,
};

type Search = Record<string, string | string[] | undefined>;

function one(v: string | string[] | undefined): string {
  return (Array.isArray(v) ? v[0] : v) ?? "";
}

function pick<T extends string>(allowed: readonly T[], v: string, fallback: T): T {
  return (allowed as readonly string[]).includes(v) ? (v as T) : fallback;
}

function nat(v: string): number {
  return /^\d{1,9}$/.test(v) ? Number(v) : 0;
}

export function parseMatchState(sp: Search): MatchListState {
  const page = one(sp.page);
  return {
    hero: nat(one(sp.hero)),
    result: pick(RESULTS, one(sp.result), ""),
    mode: nat(one(sp.mode)),
    meta: pick(METAS, one(sp.meta), ""),
    sort: pick(MATCH_SORTS, one(sp.sort), "date"),
    dir: pick(["asc", "desc"] as const, one(sp.dir), "desc"),
    // Tokens are "<int>_<id>"; anything else is ignored rather than sent.
    page: /^-?\d{1,20}_\d{1,20}$/.test(page) ? page : "",
    from: page ? nat(one(sp.from)) : 0,
  };
}

/** Query string for a state, without defaults; "" when everything is default. */
export function matchStateQuery(s: MatchListState): string {
  const q = new URLSearchParams();
  if (s.hero) q.set("hero", String(s.hero));
  if (s.result) q.set("result", s.result);
  if (s.mode) q.set("mode", String(s.mode));
  if (s.meta) q.set("meta", s.meta);
  if (s.sort !== "date" || s.dir !== "desc") {
    q.set("sort", s.sort);
    q.set("dir", s.dir);
  }
  if (s.page) {
    q.set("page", s.page);
    q.set("from", String(s.from));
  }
  const str = q.toString();
  return str ? `?${str}` : "";
}

/** Link for a sort header: same column flips, a new one starts descending. */
export function sortHref(base: string, s: MatchListState, key: MatchSortKey): string {
  const dir: SortDir = s.sort === key ? (s.dir === "desc" ? "asc" : "desc") : "desc";
  return base + matchStateQuery({ ...s, sort: key, dir, page: "", from: 0 });
}

/** Link with changed filters; any change starts from the first page. */
export function filterHref(base: string, s: MatchListState, change: Partial<MatchListState>): string {
  return base + matchStateQuery({ ...s, ...change, page: "", from: 0 });
}

export function nextPageHref(base: string, s: MatchListState, token: string, pageSize: number): string {
  return base + matchStateQuery({ ...s, page: token, from: s.from + pageSize });
}

const apiSort: Record<MatchSortKey, MatchSort> = {
  date: "MATCH_SORT_START_TIME",
  duration: "MATCH_SORT_DURATION",
  kills: "MATCH_SORT_KILLS",
  deaths: "MATCH_SORT_DEATHS",
  assists: "MATCH_SORT_ASSISTS",
  souls: "MATCH_SORT_NET_WORTH",
  lastHits: "MATCH_SORT_LAST_HITS",
  level: "MATCH_SORT_HERO_LEVEL",
};

const apiResult: Record<ResultFilter, MatchResult> = {
  "": "MATCH_RESULT_UNSPECIFIED",
  win: "MATCH_RESULT_WIN",
  loss: "MATCH_RESULT_LOSS",
};

const apiMeta: Record<MetaFilter, MetaStatus> = {
  "": "META_STATUS_UNSPECIFIED",
  ok: "META_STATUS_OK",
  pending: "META_STATUS_PENDING",
  expired: "META_STATUS_EXPIRED",
  failed: "META_STATUS_FAILED",
};

/** Query parameters of GET /v1/players/{id}/matches for this state. */
export function matchApiParams(s: MatchListState, pageSize: number): URLSearchParams {
  const q = new URLSearchParams({ pageSize: String(pageSize) });
  if (s.hero) q.set("heroId", String(s.hero));
  if (s.result) q.set("result", apiResult[s.result]);
  if (s.mode) q.set("gameMode", String(s.mode));
  if (s.meta) q.set("metaStatus", apiMeta[s.meta]);
  if (s.sort !== "date") q.set("sort", apiSort[s.sort]);
  if (s.dir === "asc") q.set("ascending", "true");
  if (s.page) q.set("pageToken", s.page);
  return q;
}
