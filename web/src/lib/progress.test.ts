import assert from "node:assert/strict";
import { join } from "node:path";
import { test } from "node:test";

import { loadFixtures } from "../../mock/api.ts";
import type { MatchFacts } from "../api/schema.gen.ts";
import {
  afterStreaks,
  aggregate,
  badgeRank,
  badgeStep,
  calendar,
  form,
  hours,
  minHeroMatches,
  months,
  quantile,
  records,
  sessions,
  speed,
  stepBadge,
  weakHours,
  weakestSessionPos,
  winRate,
} from "./progress.ts";

const T0 = 1_780_000_000; // 2026-05-28 20:26:40 UTC

function fact(over: Partial<MatchFacts> = {}): MatchFacts {
  return {
    matchId: "1",
    startTime: String(T0),
    durationS: 1800,
    gameMode: 1,
    matchMode: 1,
    heroId: 13,
    won: false,
    kills: 5,
    deaths: 5,
    assists: 5,
    netWorth: 30000,
    lastHits: 100,
    hasMeta: false,
    playerDamage: 0,
    playerHealing: 0,
    soulsAt: [],
    timeToSoulsS: [],
    soulsFrom: null,
    lobbyBadge: 0,
    ...over,
  };
}

/** n matches an hour apart (each its own session), wins where pattern[i] is "W". */
function series(pattern: string, over: (i: number) => Partial<MatchFacts> = () => ({})): MatchFacts[] {
  return [...pattern].map((c, i) =>
    fact({ matchId: String(i + 1), startTime: String(T0 + i * 7200), won: c === "W", ...over(i) }),
  );
}

test("aggregate: per-match means, short games ignored", () => {
  const a = aggregate([
    fact({ won: true, netWorth: 30000, durationS: 1800, kills: 4, deaths: 0, assists: 2 }), // 1000/min, KDA 6
    fact({ netWorth: 10000, durationS: 600, kills: 1, deaths: 2, assists: 1 }), // 1000/min, KDA 1
    fact({ durationS: 200, won: true, netWorth: 99999 }), // remake
  ]);
  assert.equal(a.matches, 2);
  assert.equal(a.winRate, 0.5);
  assert.equal(a.soulsPerMin, 1000);
  assert.equal(a.kda, 3.5);
  assert.equal(a.deaths, 1);
  assert.deepEqual(aggregate([]), { matches: 0, wins: 0, winRate: 0, soulsPerMin: 0, kda: 0, deaths: 0 });
});

test("form: latest half against the one before, trend by win rate", () => {
  const f = form(series("LLLLLLWWWWWW"), 30);
  assert.ok(f);
  assert.equal(f.size, 6);
  assert.equal(f.now.winRate, 1);
  assert.equal(f.was.winRate, 0);
  assert.equal(f.trend, "up");
  assert.equal(form(series("WLWLWLWLWLWL"))?.trend, "flat");
  assert.equal(form(series("WWWWWWLLLLLL"))?.trend, "down");
  assert.equal(form(series("WLWLWLWL")), null, "4 per half is too few");
  // Only the latest 2 × size matter.
  assert.equal(form(series("LLLLLLLLLL" + "WWWWW" + "LLLLL"), 5)?.trend, "down");
});

test("calendar respects the time zone", () => {
  assert.deepEqual(calendar(T0, "UTC"), { year: 2026, month: 5, hour: 20 });
  assert.deepEqual(calendar(T0, "Asia/Tbilisi"), { year: 2026, month: 5, hour: 0 }, "UTC+4: past midnight");
  assert.equal(calendar(Date.UTC(2026, 0, 31, 22) / 1000, "Asia/Tbilisi").month, 2, "Tbilisi is already in February");
});

test("badges: packed tier * 10 + subtier, averaged on the linear scale", () => {
  assert.deepEqual(badgeRank(72), { tier: 7, sub: 2 });
  assert.equal(badgeRank(70), null);
  assert.equal(badgeRank(0), null);
  assert.equal(badgeStep(11), 0);
  assert.equal(badgeStep(76), 41);
  assert.equal(stepBadge(41), 76);
  assert.equal(stepBadge(42), 81);
  for (const b of [11, 16, 21, 55, 116]) assert.equal(stepBadge(badgeStep(b)!), b);
});

test("months: grouped in the time zone, badge mean ignores unknown", () => {
  const day = 86400;
  const fs = [
    fact({ startTime: String(T0), won: true, lobbyBadge: 72 }),
    fact({ startTime: String(T0 + day), lobbyBadge: 81 }),
    fact({ startTime: String(T0 + 2 * day), lobbyBadge: 0 }),
    fact({ startTime: String(T0 + 40 * day) }),
    fact({ startTime: String(T0 + 41 * day), durationS: 100 }), // remake only: month dropped
  ];
  const ms = months(fs.slice(0, 4), "UTC");
  // May 28–30 and July 7.
  assert.deepEqual(
    ms.map((m) => [m.key, m.agg.matches, m.agg.wins, m.badge, m.badgeMatches]),
    [
      ["2026-05", 3, 1, 75, 2], // Archon 2 and Oracle 1 average to Archon 5
      ["2026-07", 1, 0, 0, 0],
    ],
  );
  assert.equal(months([fs[4]!], "UTC").length, 0);
});

test("quantile interpolates like percentile_cont", () => {
  assert.equal(quantile([], 0.5), 0);
  assert.equal(quantile([5], 0.9), 5);
  assert.equal(quantile([1, 2, 3, 4], 0.5), 2.5);
  assert.equal(quantile([10, 0, 5], 0.9), 9);
});

test("speed: marks, targets, sources and fastest heroes", () => {
  const src = { heroes: 100, laneCreeps: 300, neutrals: 50, bosses: 50, treasure: 0 };
  const fs = [
    fact({ hasMeta: true, heroId: 1, soulsAt: [3000, 7000], timeToSoulsS: [700, 0], soulsFrom: src }),
    fact({ hasMeta: true, heroId: 1, soulsAt: [4000, 9000], timeToSoulsS: [600, 1500], soulsFrom: src }),
    fact({ hasMeta: true, heroId: 2, soulsAt: [2000, 0], timeToSoulsS: [0, 0], soulsFrom: src }), // ended early
    fact({ hasMeta: false, soulsAt: [], heroId: 3 }),
  ];
  const s = speed(fs, [300, 600], [10000, 20000], { minHeroMatches: 2 });
  assert.equal(s.matches, 3);
  assert.deepEqual(
    s.marks.map((m) => [m.t, m.avg, m.matches]),
    [
      [300, 3000, 3],
      [600, 8000, 2],
    ],
  );
  assert.equal(s.targets[0]?.avg, 650);
  assert.equal(s.targets[0]?.reached, 2 / 3);
  assert.equal(s.targets[1]?.fast, 1500);
  assert.deepEqual(
    s.sources.map((x) => [x.key, x.share]),
    [
      ["laneCreeps", 0.6],
      ["heroes", 0.2],
      ["neutrals", 0.1],
      ["bosses", 0.1],
      ["treasure", 0],
    ],
  );
  assert.deepEqual(s.heroes, [{ heroId: 1, matches: 2, souls: 8000 }], "hero 2 never reached 10:00");
});

test("hero ranking threshold grows with the history", () => {
  assert.equal(minHeroMatches(0), 3);
  assert.equal(minHeroMatches(106), 4);
  assert.equal(minHeroMatches(678), 10);
});

test("sessions: a pause over an hour starts a new one", () => {
  const start = (min: number) => String(T0 + min * 60);
  const fs = [
    fact({ startTime: start(0), durationS: 1800, won: true }), // session 1, #1
    fact({ startTime: start(35), durationS: 1800 }), // #2: 5 min after the end
    fact({ startTime: start(70), durationS: 1800 }), // #3
    fact({ startTime: start(300), durationS: 1800, won: true }), // new session, #1
    fact({ startTime: start(335), durationS: 1800, won: true }), // #2
  ];
  assert.deepEqual(sessions(fs, 3), [
    { matches: 2, wins: 2 },
    { matches: 2, wins: 1 },
    { matches: 1, wins: 0 },
  ]);
  assert.deepEqual(sessions(fs, 2)[1], { matches: 3, wins: 1 }, "the last bucket collects the rest");
});

test("after streaks", () => {
  const r = afterStreaks(series("LLWWWLLL"));
  // after LL: W (i=2), L (i=7); after WW: W (i=4), L (i=5)
  assert.deepEqual(r.afterLosses, { matches: 2, wins: 1 });
  assert.deepEqual(r.afterWins, { matches: 2, wins: 1 });
  assert.deepEqual(r.all, { matches: 8, wins: 3 });
});

test("hours by local start time", () => {
  const h = hours([fact({ won: true }), fact({ startTime: String(T0 + 3600) })], "Asia/Tbilisi");
  assert.equal(h.length, 24);
  assert.deepEqual(h[0], { matches: 1, wins: 1 }, "20:26 UTC is 00:26 in Tbilisi");
  assert.deepEqual(h[1], { matches: 1, wins: 0 });
});

test("records: best values with their match, longest streaks", () => {
  const r = records(series("WWLLLW", (i) => ({ kills: [3, 9, 1, 9, 0, 2][i], playerDamage: i === 4 ? 5000 : 0 })));
  const kills = r.best.find((b) => b.key === "kills");
  assert.equal(kills?.value, 9);
  assert.equal(kills?.matchId, "2", "the first of equal records");
  assert.equal(r.best.find((b) => b.key === "playerDamage")?.matchId, "5");
  assert.equal(r.best.find((b) => b.key === "playerHealing"), undefined, "no healing at all: no record");
  assert.equal(r.winStreak, 2);
  assert.equal(r.lossStreak, 3);
  assert.deepEqual(records([]), { best: [], winStreak: 0, lossStreak: 0 });
});

test("insights: weakest session position and weak hours need enough matches", () => {
  const b = (matches: number, wins: number) => ({ matches, wins });
  assert.equal(weakestSessionPos([b(100, 50), b(80, 35), b(40, 19), b(10, 0)]), 1, "10 matches is too few");
  assert.equal(weakestSessionPos([b(100, 50), b(80, 39)]), null, "1 p.p. is noise");
  assert.equal(weakestSessionPos([b(10, 5), b(80, 10)]), null);
  const hs = Array.from({ length: 24 }, () => b(0, 0));
  hs[16] = b(25, 5);
  hs[18] = b(60, 22);
  hs[21] = b(270, 134);
  hs[11] = b(19, 0);
  assert.deepEqual(weakHours(hs, 0.475), [16, 18]);
  assert.equal(winRate(b(4, 1)), 0.25);
  assert.equal(winRate(b(0, 0)), 0);
});

test("real fixtures: speed and months are consistent", () => {
  const fx = loadFixtures(join(import.meta.dirname, "..", "..", "mock", "fixtures"));
  const { matches, soulMarksS, soulTargets } = fx.facts;
  assert.ok(matches.length > 0);
  const starts = matches.map((f) => Number(f.startTime));
  assert.deepEqual(starts, [...starts].sort((a, b) => a - b), "oldest first");
  const s = speed(matches, soulMarksS, soulTargets);
  for (let i = 1; i < s.marks.length; i++) assert.ok(s.marks[i]!.avg > s.marks[i - 1]!.avg, "souls grow");
  for (const m of s.marks) assert.ok(m.top >= m.avg);
  for (const t of s.targets) assert.ok(t.fast <= t.avg);
  assert.ok(Math.abs(s.sources.reduce((x, y) => x + y.share, 0) - 1) < 1e-9);
  const ms = months(matches, "Asia/Tbilisi");
  assert.equal(
    ms.reduce((x, m) => x + m.agg.matches, 0),
    aggregate(matches).matches,
  );
  for (const m of ms) if (m.badge) assert.ok(badgeRank(m.badge));
});
