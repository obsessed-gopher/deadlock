import type { HeroStats } from "../api/schema.gen.ts";

export interface HeroRow {
  heroId: number;
  matches: number;
  wins: number;
  /** 0..1 */
  winRate: number;
  avgKills: number;
  avgDeaths: number;
  avgAssists: number;
  /** (kills + assists) / deaths over all matches, deaths at least 1. */
  kda: number;
  soulsPerMin: number;
  lastPlayed: string;
}

/** Per-hero averages from the totals returned by ListPlayerHeroStats. */
export function heroRows(stats: HeroStats[]): HeroRow[] {
  return stats
    .filter((h) => h.matches > 0)
    .map((h) => ({
      heroId: h.heroId,
      matches: h.matches,
      wins: h.wins,
      winRate: h.wins / h.matches,
      avgKills: h.kills / h.matches,
      avgDeaths: h.deaths / h.matches,
      avgAssists: h.assists / h.matches,
      kda: (h.kills + h.assists) / Math.max(1, h.deaths),
      soulsPerMin: h.durationS > 0 ? h.netWorth / (h.durationS / 60) : 0,
      lastPlayed: h.lastPlayed,
    }));
}
