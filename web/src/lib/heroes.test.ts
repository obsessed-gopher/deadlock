import assert from "node:assert/strict";
import { test } from "node:test";

import type { HeroStats } from "../api/schema.gen.ts";
import { heroRows } from "./heroes.ts";

function stats(over: Partial<HeroStats>): HeroStats {
  return {
    heroId: 13,
    matches: 4,
    wins: 3,
    kills: 20,
    deaths: 8,
    assists: 12,
    netWorth: 120000,
    lastHits: 400,
    durationS: 7200,
    lastPlayed: "1794330963",
    ...over,
  };
}

test("averages per match", () => {
  const [r] = heroRows([stats({})]);
  assert.equal(r?.winRate, 0.75);
  assert.equal(r?.avgKills, 5);
  assert.equal(r?.avgDeaths, 2);
  assert.equal(r?.avgAssists, 3);
  assert.equal(r?.kda, 4);
  assert.equal(r?.soulsPerMin, 1000);
});

test("no deaths and no duration do not divide by zero", () => {
  const [r] = heroRows([stats({ deaths: 0, durationS: 0 })]);
  assert.equal(r?.kda, 32);
  assert.equal(r?.soulsPerMin, 0);
});

test("heroes without matches are dropped, order is kept", () => {
  const rows = heroRows([stats({ heroId: 1 }), stats({ heroId: 2, matches: 0 }), stats({ heroId: 3 })]);
  assert.deepEqual(
    rows.map((r) => r.heroId),
    [1, 3],
  );
});
