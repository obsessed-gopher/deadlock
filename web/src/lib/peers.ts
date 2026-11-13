// "Who you play with": pure functions over ListPlayerPeersResponse. Wins are
// always the profile owner's wins, counted over matches with metadata only.
import type { HeroMatchup, PartySize, Peer } from "../api/schema.gen.ts";
import { type Bucket, winRate } from "./progress.ts";

/** "Without X" needs this many matches to be a fair comparison. */
export const MIN_WITHOUT = 10;

/** Fewer shared matches than this: show the numbers, but don't colour them. */
export const MIN_SAMPLE = 10;

/** A change smaller than this (5 percentage points) is noise, not a finding. */
export const NOTABLE = 0.05;

export interface WithWithout {
  with: Bucket;
  without: Bucket;
  /** with − without win rate; null when either side is too small. */
  delta: number | null;
}

/** The owner's win rate with someone and in all other matches. */
export function withWithout(peer: Bucket, total: Bucket): WithWithout {
  const without = { matches: total.matches - peer.matches, wins: total.wins - peer.wins };
  const delta = without.matches >= MIN_WITHOUT && peer.matches > 0 ? winRate(peer) - winRate(without) : null;
  return { with: { matches: peer.matches, wins: peer.wins }, without, delta };
}

/** Allies met often enough to be queue mates, most matches first. */
export function friends(allies: Peer[], minMatches: number): Peer[] {
  return allies.filter((a) => a.matches >= minMatches);
}

export interface Party {
  solo: Bucket;
  /** Any friend on the team. */
  party: Bucket;
  /** Index = friends on the team, 0..last reported size, gaps as zeros. */
  bySize: Bucket[];
}

export function party(sizes: PartySize[]): Party {
  const max = Math.max(0, ...sizes.map((s) => s.friends));
  const bySize: Bucket[] = Array.from({ length: max + 1 }, () => ({ matches: 0, wins: 0 }));
  for (const s of sizes) bySize[s.friends] = { matches: s.matches, wins: s.wins };
  const party = bySize.slice(1).reduce((a, b) => ({ matches: a.matches + b.matches, wins: a.wins + b.wins }), {
    matches: 0,
    wins: 0,
  });
  return { solo: bySize[0]!, party, bySize };
}

/**
 * The party size (friends on the team, 1+) with the lowest win rate, when it
 * has enough matches and is notably below solo.
 */
export function weakestParty(p: Party, minMatches: number): { friends: number; bucket: Bucket } | null {
  let worst: { friends: number; bucket: Bucket } | null = null;
  p.bySize.forEach((b, n) => {
    if (n === 0 || b.matches < minMatches || p.solo.matches < minMatches) return;
    if (winRate(b) > winRate(p.solo) - NOTABLE) return;
    if (!worst || winRate(b) < winRate(worst.bucket)) worst = { friends: n, bucket: b };
  });
  return worst;
}

/** Matches a hero must be seen in before its win rate means anything. */
export function minMatchup(totalMatches: number): number {
  return Math.max(5, Math.min(30, Math.round(totalMatches / 20)));
}

export interface HeroEdge {
  heroId: number;
  matches: number;
  wins: number;
  /** Win rate minus the overall one. */
  delta: number;
}

export interface HeroEdges {
  /** Enemy heroes you lose to most, worst first. */
  hardest: HeroEdge[];
  /** Enemy heroes you beat most, best first. */
  easiest: HeroEdge[];
  /** Allied heroes you win with most, best first. */
  bestAllies: HeroEdge[];
}

/** Notable hero matchups: at least `min` matches and NOTABLE away from overall. */
export function heroEdges(heroes: HeroMatchup[], total: Bucket, min: number, top = 3): HeroEdges {
  const overall = winRate(total);
  const edges = (side: "with" | "against") =>
    heroes
      .map((h) => {
        const b = side === "with" ? { matches: h.withMatches, wins: h.withWins } : { matches: h.againstMatches, wins: h.againstWins };
        return { heroId: h.heroId, ...b, delta: winRate(b) - overall };
      })
      .filter((e) => e.matches >= min);
  const asc = (a: HeroEdge, b: HeroEdge) => a.delta - b.delta || b.matches - a.matches || a.heroId - b.heroId;
  const desc = (a: HeroEdge, b: HeroEdge) => b.delta - a.delta || b.matches - a.matches || a.heroId - b.heroId;
  const against = edges("against");
  return {
    hardest: against.filter((e) => e.delta <= -NOTABLE).sort(asc).slice(0, top),
    easiest: against.filter((e) => e.delta >= NOTABLE).sort(desc).slice(0, top),
    bestAllies: edges("with").filter((e) => e.delta >= NOTABLE).sort(desc).slice(0, top),
  };
}

/** The friend whose presence moves the owner's win rate most, either way. */
export function biggestImpact(fs: Peer[], total: Bucket): { peer: Peer; delta: number } | null {
  let best: { peer: Peer; delta: number } | null = null;
  for (const p of fs) {
    if (p.matches < MIN_SAMPLE) continue;
    const { delta } = withWithout(p, total);
    if (delta === null || Math.abs(delta) < NOTABLE) continue;
    if (!best || Math.abs(delta) > Math.abs(best.delta)) best = { peer: p, delta };
  }
  return best;
}
