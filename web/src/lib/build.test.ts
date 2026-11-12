import assert from "node:assert/strict";
import { test } from "node:test";

import type { Ability, ItemEvent } from "../api/schema.gen.ts";
import { buildEntries, finalItems } from "./build.ts";

const abilities = new Map<number, Ability>([
  [1, { id: 1, key: "ability_smoke_bomb", name: "Smoke Bomb", nameRu: "Дымовая бомба", type: "signature", slot: "", tier: 0 }],
  [2, { id: 2, key: "upgrade_rapid_rounds", name: "Rapid Rounds", nameRu: "", type: "item", slot: "weapon", tier: 1 }],
  [3, { id: 3, key: "upgrade_magic_storm", name: "Magic Storm", nameRu: "", type: "item", slot: "spirit", tier: 4 }],
]);
const ability = (id: number) => abilities.get(id);

function ev(t: number, itemId: number, upgradeId = 0, soldT = 0): ItemEvent {
  return { t, itemId, upgradeId, soldT, flags: 0 };
}

test("ability upgrades are numbered by order, not by upgrade_id", () => {
  // Shapes from match 134969900: unlock with upgrade_id 0, then hashes.
  const got = buildEntries([ev(11, 1), ev(1070, 1, 582914516), ev(1252, 1, 2018283944)], ability);
  assert.deepEqual(
    got.map((e) => [e.kind, e.level, e.name]),
    [
      ["unlock", 0, "Дымовая бомба"],
      ["upgrade", 1, "Дымовая бомба"],
      ["upgrade", 2, "Дымовая бомба"],
    ],
  );
});

test("shop items keep tier, slot and sale time; unknown ids get a fallback name", () => {
  const [item, unknown] = buildEntries([ev(203, 2, 1, 708), ev(300, 999)], ability);
  assert.deepEqual(item, {
    t: 203,
    itemId: 2,
    name: "Rapid Rounds",
    kind: "item",
    level: 0,
    tier: 1,
    slot: "weapon",
    soldT: 708,
  });
  assert.equal(unknown?.name, "#999");
  assert.equal(unknown?.kind, "item");
});

test("events are ordered by time", () => {
  const got = buildEntries([ev(500, 2), ev(100, 3)], ability);
  assert.deepEqual(
    got.map((e) => e.t),
    [100, 500],
  );
});

test("finalItems drops sold items and abilities, sorts by tier", () => {
  const entries = buildEntries([ev(11, 1), ev(203, 2, 1, 708), ev(250, 2), ev(1371, 3)], ability);
  assert.deepEqual(
    finalItems(entries).map((e) => [e.itemId, e.t]),
    [
      [3, 1371],
      [2, 250],
    ],
  );
});
