// Player progress from match facts (ListPlayerMatchFacts): form, monthly
// trends, development speed, sessions, records. Pure functions over the
// facts, oldest first; the page localizes and draws the results.
import type { MatchFacts } from "../api/schema.gen.ts";
import { kdaRatio } from "./format.ts";

/** Shorter games (remakes, early leaves) distort per-minute numbers. */
export const MIN_DURATION_S = 300;

export const usable = (f: MatchFacts) => f.durationS >= MIN_DURATION_S;

export interface Agg {
  matches: number;
  wins: number;
  /** 0..1; 0 when there are no matches. */
  winRate: number;
  soulsPerMin: number;
  kda: number;
  deaths: number;
}

const mean = (xs: number[]) => (xs.length ? xs.reduce((a, b) => a + b, 0) / xs.length : 0);

/** Averages per match, like the Go hero stats: KDA of each match, then mean. */
export function aggregate(facts: MatchFacts[]): Agg {
  const fs = facts.filter(usable);
  const wins = fs.filter((f) => f.won).length;
  return {
    matches: fs.length,
    wins,
    winRate: fs.length ? wins / fs.length : 0,
    soulsPerMin: mean(fs.map((f) => (f.netWorth * 60) / f.durationS)),
    kda: mean(fs.map((f) => kdaRatio(f.kills, f.deaths, f.assists))),
    deaths: mean(fs.map((f) => f.deaths)),
  };
}

export interface Form {
  /** Matches in each half. */
  size: number;
  now: Agg;
  was: Agg;
  trend: "up" | "down" | "flat";
}

/** Win rate change that counts as a trend, not noise. */
export const TREND_WIN_RATE = 0.05;

/**
 * The latest `size` usable matches against the `size` before them; fewer
 * matches split in half. Null below 5 matches per half: too noisy.
 */
export function form(facts: MatchFacts[], size = 30): Form | null {
  const fs = facts.filter(usable);
  const n = Math.min(size, Math.floor(fs.length / 2));
  if (n < 5) return null;
  const now = aggregate(fs.slice(-n));
  const was = aggregate(fs.slice(-2 * n, -n));
  const d = now.winRate - was.winRate;
  return { size: n, now, was, trend: d >= TREND_WIN_RATE ? "up" : d <= -TREND_WIN_RATE ? "down" : "flat" };
}

// ---------- calendar ----------

const partsFmt = new Map<string, Intl.DateTimeFormat>();

/** Calendar parts of a unix time in a time zone (dates render on the server). */
export function calendar(unix: string | number, tz: string): { year: number; month: number; hour: number } {
  let fmt = partsFmt.get(tz);
  if (!fmt) {
    fmt = new Intl.DateTimeFormat("en-GB", {
      timeZone: tz,
      year: "numeric",
      month: "numeric",
      hour: "numeric",
      hourCycle: "h23",
    });
    partsFmt.set(tz, fmt);
  }
  const p = Object.fromEntries(fmt.formatToParts(new Date(Number(unix) * 1000)).map((x) => [x.type, x.value]));
  return { year: Number(p.year), month: Number(p.month), hour: Number(p.hour) };
}

// ---------- rank badges ----------

export const SUBTIERS = 6;

/** Packed badge (tier * 10 + subtier) -> tier 1..11 and subtier 1..6; null if invalid. */
export function badgeRank(badge: number): { tier: number; sub: number } | null {
  const tier = Math.floor(badge / 10);
  const sub = badge % 10;
  return tier >= 1 && sub >= 1 && sub <= SUBTIERS ? { tier, sub } : null;
}

/** Position on the linear rank scale (0 = tier 1 subtier 1); null if invalid. */
export function badgeStep(badge: number): number | null {
  const r = badgeRank(badge);
  return r ? (r.tier - 1) * SUBTIERS + r.sub - 1 : null;
}

export function stepBadge(step: number): number {
  const s = Math.max(0, Math.round(step));
  return (Math.floor(s / SUBTIERS) + 1) * 10 + (s % SUBTIERS) + 1;
}

// ---------- months ----------

export interface Month {
  /** "2026-03" */
  key: string;
  year: number;
  month: number;
  agg: Agg;
  /** Mean lobby badge on the linear scale, as a packed badge; 0 if none. */
  badge: number;
  badgeMatches: number;
}

export function months(facts: MatchFacts[], tz: string): Month[] {
  const by = new Map<string, { year: number; month: number; fs: MatchFacts[] }>();
  for (const f of facts) {
    const { year, month } = calendar(f.startTime, tz);
    const key = `${year}-${String(month).padStart(2, "0")}`;
    const m = by.get(key) ?? { year, month, fs: [] };
    m.fs.push(f);
    by.set(key, m);
  }
  return [...by.entries()]
    .sort(([a], [b]) => (a < b ? -1 : 1))
    .map(([key, { year, month, fs }]) => {
      const steps = fs.map((f) => badgeStep(f.lobbyBadge)).filter((s): s is number => s !== null);
      return {
        key,
        year,
        month,
        agg: aggregate(fs),
        badge: steps.length ? stepBadge(mean(steps)) : 0,
        badgeMatches: steps.length,
      };
    })
    .filter((m) => m.agg.matches > 0);
}

// ---------- development speed ----------

/** Linear-interpolated quantile of unsorted values (Postgres percentile_cont). */
export function quantile(values: number[], q: number): number {
  if (values.length === 0) return 0;
  const s = [...values].sort((a, b) => a - b);
  const pos = (s.length - 1) * q;
  const lo = Math.floor(pos);
  const hi = Math.ceil(pos);
  return s[lo]! + (s[hi]! - s[lo]!) * (pos - lo);
}

export interface SoulMark {
  /** Game time, s. */
  t: number;
  avg: number;
  /** Souls in the best 10% of matches. */
  top: number;
  matches: number;
}

export interface SoulTarget {
  souls: number;
  /** Mean time to reach it, s, over the matches that did. */
  avg: number;
  /** Time in the fastest 10% of matches. */
  fast: number;
  /** Share of matches with metadata that reached it. */
  reached: number;
}

export const SOURCE_KEYS = ["laneCreeps", "heroes", "neutrals", "bosses", "treasure"] as const;
export type SourceKey = (typeof SOURCE_KEYS)[number];

export interface HeroSpeed {
  heroId: number;
  matches: number;
  /** Mean souls at the hero mark (10:00). */
  souls: number;
}

export interface Speed {
  matches: number;
  marks: SoulMark[];
  targets: SoulTarget[];
  sources: { key: SourceKey; souls: number; share: number }[];
  heroes: HeroSpeed[];
  heroMark: number;
}

/** Matches on a hero to rank its start: 10, fewer while the history is short. */
export function minHeroMatches(matchesWithMeta: number): number {
  return Math.max(3, Math.min(10, Math.round(matchesWithMeta / 30)));
}

export function speed(
  facts: MatchFacts[],
  marksS: number[],
  targets: number[],
  { heroMark = 600, minHeroMatches = 10 } = {},
): Speed {
  const fs = facts.filter((f) => f.hasMeta);
  const marks = marksS.map((t, i) => {
    const v = fs.map((f) => f.soulsAt[i] ?? 0).filter((x) => x > 0);
    return { t, avg: mean(v), top: quantile(v, 0.9), matches: v.length };
  });
  const tgs = targets.map((souls, i) => {
    const v = fs.map((f) => f.timeToSoulsS[i] ?? 0).filter((x) => x > 0);
    return { souls, avg: mean(v), fast: quantile(v, 0.1), reached: fs.length ? v.length / fs.length : 0 };
  });
  const totals = Object.fromEntries(SOURCE_KEYS.map((k) => [k, 0])) as Record<SourceKey, number>;
  for (const f of fs) for (const k of SOURCE_KEYS) totals[k] += f.soulsFrom?.[k] ?? 0;
  const all = SOURCE_KEYS.reduce((s, k) => s + totals[k], 0);

  const mi = marksS.indexOf(heroMark);
  const byHero = new Map<number, number[]>();
  if (mi >= 0) {
    for (const f of fs) {
      const v = f.soulsAt[mi] ?? 0;
      if (v > 0) byHero.set(f.heroId, [...(byHero.get(f.heroId) ?? []), v]);
    }
  }
  const heroes = [...byHero.entries()]
    .filter(([, v]) => v.length >= minHeroMatches)
    .map(([heroId, v]) => ({ heroId, matches: v.length, souls: mean(v) }))
    .sort((a, b) => b.souls - a.souls || a.heroId - b.heroId);

  return {
    matches: fs.length,
    marks,
    targets: tgs,
    sources: SOURCE_KEYS.map((key) => ({ key, souls: totals[key], share: all ? totals[key] / all : 0 })).sort(
      (a, b) => b.souls - a.souls,
    ),
    heroes,
    heroMark,
  };
}

// ---------- sessions and time ----------

export interface Bucket {
  matches: number;
  wins: number;
}

const rate = (b: Bucket) => (b.matches ? b.wins / b.matches : 0);
export { rate as winRate };

/** A pause longer than this between matches starts a new session. */
export const SESSION_GAP_S = 3600;

/**
 * Win rate by the position of a match in its session: index 0 is the first
 * match, the last bucket collects matches from `maxPos` on.
 */
export function sessions(facts: MatchFacts[], maxPos = 5, gapS = SESSION_GAP_S): Bucket[] {
  const out: Bucket[] = Array.from({ length: maxPos }, () => ({ matches: 0, wins: 0 }));
  let prevEnd = -Infinity;
  let pos = 0;
  for (const f of facts) {
    const start = Number(f.startTime);
    pos = start - prevEnd > gapS ? 0 : pos + 1;
    prevEnd = start + f.durationS;
    const b = out[Math.min(pos, maxPos - 1)]!;
    b.matches++;
    b.wins += f.won ? 1 : 0;
  }
  return out;
}

/** Results right after `n` losses or `n` wins in a row. */
export function afterStreaks(facts: MatchFacts[], n = 2): { afterLosses: Bucket; afterWins: Bucket; all: Bucket } {
  const afterLosses = { matches: 0, wins: 0 };
  const afterWins = { matches: 0, wins: 0 };
  facts.forEach((f, i) => {
    if (i < n) return;
    const prev = facts.slice(i - n, i);
    const b = prev.every((p) => !p.won) ? afterLosses : prev.every((p) => p.won) ? afterWins : null;
    if (!b) return;
    b.matches++;
    b.wins += f.won ? 1 : 0;
  });
  return { afterLosses, afterWins, all: { matches: facts.length, wins: facts.filter((f) => f.won).length } };
}

/** 24 buckets by the local hour the match started. */
export function hours(facts: MatchFacts[], tz: string): Bucket[] {
  const out: Bucket[] = Array.from({ length: 24 }, () => ({ matches: 0, wins: 0 }));
  for (const f of facts) {
    const b = out[calendar(f.startTime, tz).hour]!;
    b.matches++;
    b.wins += f.won ? 1 : 0;
  }
  return out;
}

// ---------- records ----------

export const RECORD_KEYS = ["kills", "netWorth", "playerDamage", "playerHealing", "assists", "lastHits"] as const;
export type RecordKey = (typeof RECORD_KEYS)[number];

export interface Best {
  key: RecordKey;
  value: number;
  matchId: string;
  heroId: number;
  startTime: string;
}

export interface Records {
  best: Best[];
  winStreak: number;
  lossStreak: number;
}

export function records(facts: MatchFacts[]): Records {
  const best: Best[] = [];
  for (const key of RECORD_KEYS) {
    let top: MatchFacts | null = null;
    for (const f of facts) if (f[key] > 0 && (!top || f[key] > top[key])) top = f;
    if (top) best.push({ key, value: top[key], matchId: top.matchId, heroId: top.heroId, startTime: top.startTime });
  }
  let win = 0;
  let loss = 0;
  let cur = 0;
  let prev: boolean | null = null;
  for (const f of facts) {
    cur = f.won === prev ? cur + 1 : 1;
    prev = f.won;
    if (f.won) win = Math.max(win, cur);
    else loss = Math.max(loss, cur);
  }
  return { best, winStreak: win, lossStreak: loss };
}

// ---------- insights ----------

/** Fewest matches in a bucket to say anything about it. */
export const MIN_BUCKET = 20;

/** The session position with the lowest win rate, if clearly below the first match. */
export function weakestSessionPos(buckets: Bucket[], margin = 0.03): number | null {
  const first = buckets[0];
  if (!first || first.matches < MIN_BUCKET) return null;
  let worst: number | null = null;
  buckets.forEach((b, i) => {
    if (i === 0 || b.matches < MIN_BUCKET) return;
    if (worst === null || rate(b) < rate(buckets[worst]!)) worst = i;
  });
  return worst !== null && rate(first) - rate(buckets[worst]!) >= margin ? worst : null;
}

/** Hours that are clearly worse than overall: at least MIN_BUCKET matches, worst first. */
export function weakHours(buckets: Bucket[], overall: number, margin = 0.08, limit = 2): number[] {
  return buckets
    .map((b, hour) => ({ hour, b }))
    .filter(({ b }) => b.matches >= MIN_BUCKET && rate(b) <= overall - margin)
    .sort((a, b) => rate(a.b) - rate(b.b))
    .slice(0, limit)
    .map((x) => x.hour);
}
