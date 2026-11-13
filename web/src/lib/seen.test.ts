import assert from "node:assert/strict";
import { test } from "node:test";

import type { SeenMatch } from "../api/schema.gen.ts";
import { heroLines, inQueue, shared, summarize } from "./seen.ts";

let next = 100;
function seen(over: Partial<SeenMatch> = {}): SeenMatch {
  next++;
  return {
    matchId: String(next),
    startTime: String(1_780_000_000 + next * 3600),
    durationS: 1800,
    gameMode: 1,
    matchMode: 1,
    heroId: 1,
    team: 0,
    won: false,
    kills: 0,
    deaths: 0,
    assists: 0,
    netWorth: 0,
    lastHits: 0,
    playerDamage: 0,
    playerHealing: 0,
    level: 30,
    ...over,
  };
}

test("summarize: averages per match, rates per minute over all time", () => {
  const s = summarize([
    seen({ won: true, kills: 10, deaths: 2, assists: 4, netWorth: 30000, playerDamage: 18000 }),
    seen({ kills: 2, deaths: 6, assists: 8, netWorth: 30000 }),
  ]);
  assert.equal(s.matches, 2);
  assert.equal(s.wins, 1);
  assert.equal(s.kills, 6);
  assert.equal(s.deaths, 4);
  assert.equal(s.kda, 24 / 8);
  assert.equal(s.soulsPerMin, 1000);
  assert.equal(s.damagePerMin, 300);
  assert.equal(s.duration, 1800);
  assert.equal(summarize([]).kda, 0);
});

test("heroLines: most played first", () => {
  const lines = heroLines([seen({ heroId: 2 }), seen({ heroId: 3, won: true }), seen({ heroId: 2, won: true })]);
  assert.deepEqual(
    lines.map((l) => [l.heroId, l.matches, l.wins]),
    [
      [2, 2, 1],
      [3, 1, 1],
    ],
  );
});

test("shared: together vs against from A's side, newest first", () => {
  const m1 = seen({ team: 0, won: true });
  const m2 = seen({ team: 0, won: false });
  const m3 = seen({ team: 1, won: true, gameMode: 4 });
  const onlyA = seen();
  const a = [m1, m2, m3, onlyA];
  const b = [
    { ...m1, heroId: 9 },
    { ...m2, team: 1, won: true },
    { ...m3, team: 1 },
  ];
  const s = shared(a, b);
  assert.deepEqual(
    s.matches.map((m) => [m.matchId, m.together, m.queue]),
    [
      [m3.matchId, true, "brawl"],
      [m2.matchId, false, "normal"],
      [m1.matchId, true, "normal"],
    ],
  );
  assert.deepEqual(s.together, { matches: 2, wins: 2 });
  assert.deepEqual(s.against, { matches: 1, wins: 0 });
  assert.equal(s.matches.at(-1)?.b.heroId, 9);
});

test("inQueue keeps one queue", () => {
  const ms = [seen(), seen({ matchMode: 4 }), seen({ gameMode: 4 })];
  assert.equal(inQueue(ms, "").length, 3);
  assert.equal(inQueue(ms, "ranked").length, 1);
  assert.equal(inQueue(ms, "normal").length, 1);
});
