import type { Ability, ItemEvent } from "../api/schema.gen.ts";

export type BuildKind = "item" | "unlock" | "upgrade";

export interface BuildEntry {
  t: number;
  itemId: number;
  name: string;
  kind: BuildKind;
  /** Ability upgrade point number (1..3) for kind "upgrade", else 0. */
  level: number;
  /** Shop item tier and slot (weapon | vitality | spirit); 0 / "" otherwise. */
  tier: number;
  slot: string;
  /** Game time the item was sold, 0 if kept. */
  soldT: number;
}

/**
 * Turns raw purchase events into a readable build.
 *
 * The same event stream carries shop items and hero abilities. For abilities
 * the first event (upgrade_id 0) unlocks the ability and every later one is
 * an upgrade point; their upgrade_id is a hash that is not in the game data,
 * so the upgrade level comes from the order of events, not from the value.
 * See docs/problems.md.
 */
export function buildEntries(
  items: ItemEvent[],
  ability: (id: number) => Ability | undefined,
  nameOf: (a: Ability) => string,
): BuildEntry[] {
  const upgrades = new Map<number, number>();
  const out: BuildEntry[] = [];
  for (const ev of [...items].sort((a, b) => a.t - b.t)) {
    const a = ability(ev.itemId);
    const name = a ? nameOf(a) : `#${ev.itemId}`;
    if (a && a.type !== "item") {
      if (ev.upgradeId === 0) {
        out.push({ t: ev.t, itemId: ev.itemId, name, kind: "unlock", level: 0, tier: 0, slot: "", soldT: 0 });
      } else {
        const level = (upgrades.get(ev.itemId) ?? 0) + 1;
        upgrades.set(ev.itemId, level);
        out.push({ t: ev.t, itemId: ev.itemId, name, kind: "upgrade", level, tier: 0, slot: "", soldT: 0 });
      }
      continue;
    }
    out.push({
      t: ev.t,
      itemId: ev.itemId,
      name,
      kind: "item",
      level: 0,
      tier: a?.tier ?? 0,
      slot: a?.slot ?? "",
      soldT: ev.soldT,
    });
  }
  return out;
}

/** Shop items still owned at the end of the match, highest tier first. */
export function finalItems(entries: BuildEntry[]): BuildEntry[] {
  return entries
    .filter((e) => e.kind === "item" && e.soldT === 0)
    .sort((a, b) => b.tier - a.tier || a.t - b.t);
}
