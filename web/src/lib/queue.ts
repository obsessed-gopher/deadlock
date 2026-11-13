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
