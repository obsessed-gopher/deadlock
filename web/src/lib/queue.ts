// The queue a match was played in, as players name modes. Valve keeps ranked
// as a match mode of the normal game mode, so the game mode alone mixes
// ranked into normal. Same rule as db.queueOf in Go.
import type { MatchQueue } from "../api/schema.gen.ts";

export const QUEUES = ["normal", "ranked", "brawl", "other"] as const;
export type QueueKey = (typeof QUEUES)[number];

const GAME_NORMAL = 1;
const GAME_STREET_BRAWL = 4;
const MATCH_UNRANKED = 1;
const MATCH_RANKED = 4;
const MATCH_PLACEMENT = 8;

export function queueOf(gameMode: number, matchMode: number): QueueKey {
  if (gameMode === GAME_STREET_BRAWL) return "brawl";
  if (gameMode === GAME_NORMAL && matchMode === MATCH_RANKED) return "ranked";
  if (gameMode === GAME_NORMAL && (matchMode === MATCH_UNRANKED || matchMode === MATCH_PLACEMENT)) return "normal";
  return "other";
}

export const apiQueue: Record<QueueKey, MatchQueue> = {
  normal: "MATCH_QUEUE_NORMAL",
  ranked: "MATCH_QUEUE_RANKED",
  brawl: "MATCH_QUEUE_STREET_BRAWL",
  other: "MATCH_QUEUE_OTHER",
};

/** Matches per queue. */
export function countQueues(ms: { gameMode: number; matchMode: number }[]): Record<QueueKey, number> {
  const out: Record<QueueKey, number> = { normal: 0, ranked: 0, brawl: 0, other: 0 };
  for (const m of ms) out[queueOf(m.gameMode, m.matchMode)]++;
  return out;
}

/** A queue from a URL parameter; anything else is "" (all queues). */
export function parseQueue(v: unknown): QueueKey | "" {
  return typeof v === "string" && (QUEUES as readonly string[]).includes(v) ? (v as QueueKey) : "";
}
