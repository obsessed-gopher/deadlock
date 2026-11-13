import assert from "node:assert/strict";
import { test } from "node:test";

import type { HeroMatchup, Peer } from "../api/schema.gen.ts";
import { biggestImpact, friends, heroEdges, minMatchup, party, weakestParty, withWithout } from "./peers.ts";

function peer(id: number, matches: number, wins: number): Peer {
  return {
    accountId: String(id),
    matches,
    wins,
    firstPlayed: "0",
    lastPlayed: "0",
    lastMatchId: "1",
    topHeroId: 1,
    steamFriend: false,
  };
}

function hero(heroId: number, over: Partial<HeroMatchup>): HeroMatchup {
  return { heroId, withMatches: 0, withWins: 0, againstMatches: 0, againstWins: 0, ...over };
}

test("withWithout compares to the rest of the matches", () => {
  const w = withWithout({ matches: 40, wins: 16 }, { matches: 100, wins: 50 });
  assert.deepEqual(w.without, { matches: 60, wins: 34 });
  assert.ok(Math.abs(w.delta! - (0.4 - 34 / 60)) < 1e-9);
  // Almost every match with them: "without" is too small to compare.
  assert.equal(withWithout({ matches: 95, wins: 50 }, { matches: 100, wins: 52 }).delta, null);
});

test("friends keeps allies met often enough", () => {
  const got = friends([peer(1, 9, 4), peer(2, 5, 1), peer(3, 4, 4)], 5);
  assert.deepEqual(
    got.map((p) => p.accountId),
    ["1", "2"],
  );
});

test("party splits solo and with friends, filling gaps", () => {
  const p = party([
    { friends: 0, matches: 10, wins: 6 },
    { friends: 2, matches: 4, wins: 1 },
  ]);
  assert.deepEqual(p.solo, { matches: 10, wins: 6 });
  assert.deepEqual(p.party, { matches: 4, wins: 1 });
  assert.deepEqual(p.bySize, [
    { matches: 10, wins: 6 },
    { matches: 0, wins: 0 },
    { matches: 4, wins: 1 },
  ]);
  assert.deepEqual(party([]).bySize, [{ matches: 0, wins: 0 }]);
});

test("minMatchup scales with the sample", () => {
  assert.equal(minMatchup(20), 5);
  assert.equal(minMatchup(300), 15);
  assert.equal(minMatchup(5000), 30);
});

test("heroEdges keeps notable matchups with enough matches", () => {
  const total = { matches: 100, wins: 50 };
  const edges = heroEdges(
    [
      hero(1, { againstMatches: 20, againstWins: 6 }), // 30%: hardest
      hero(2, { againstMatches: 20, againstWins: 8 }), // 40%
      hero(3, { againstMatches: 4, againstWins: 0 }), // too few
      hero(4, { againstMatches: 20, againstWins: 10 }), // at overall: not notable
      hero(5, { againstMatches: 20, againstWins: 14 }), // 70%: easiest
      hero(6, { withMatches: 20, withWins: 13 }), // 65% alongside
      hero(7, { withMatches: 20, withWins: 11 }), // 55%: +5 pp is the edge
    ],
    total,
    5,
  );
  assert.deepEqual(
    edges.hardest.map((e) => e.heroId),
    [1, 2],
  );
  assert.deepEqual(
    edges.easiest.map((e) => e.heroId),
    [5],
  );
  assert.deepEqual(
    edges.bestAllies.map((e) => e.heroId),
    [6, 7],
  );
  assert.ok(Math.abs(edges.hardest[0]!.delta + 0.2) < 1e-9);
});

test("biggestImpact picks the largest notable change", () => {
  const total = { matches: 200, wins: 100 };
  const got = biggestImpact([peer(1, 50, 26), peer(2, 40, 12), peer(3, 30, 18)], total);
  assert.equal(got?.peer.accountId, "2"); // 30% with vs 55% without
  assert.equal(biggestImpact([peer(1, 50, 25)], total), null);
  // A friend met a few times is noise, however large the gap.
  assert.equal(biggestImpact([peer(4, 6, 0)], total), null);
});

test("weakestParty finds the party size notably below solo", () => {
  const p = party([
    { friends: 0, matches: 50, wins: 25 },
    { friends: 1, matches: 400, wins: 190 }, // 47.5%: within noise
    { friends: 2, matches: 120, wins: 50 }, // 41.7%
    { friends: 3, matches: 8, wins: 1 }, // too few
  ]);
  assert.deepEqual(weakestParty(p, 20), { friends: 2, bucket: { matches: 120, wins: 50 } });
  assert.equal(weakestParty(party([{ friends: 0, matches: 10, wins: 5 }, { friends: 1, matches: 100, wins: 10 }]), 20), null);
});
