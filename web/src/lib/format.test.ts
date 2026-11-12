import assert from "node:assert/strict";
import { test } from "node:test";

import { dateTime, duration, gameModeName, kdaRatio, num, pct, teamName } from "./format.ts";

test("duration", () => {
  assert.equal(duration(0), "0:00");
  assert.equal(duration(75), "1:15");
  assert.equal(duration(1728), "28:48");
  assert.equal(duration(3725), "1:02:05");
  assert.equal(duration(-5), "0:00");
});

test("dateTime treats zero as never", () => {
  assert.equal(dateTime("0"), "—");
  assert.match(dateTime("1794330963"), /^\d{2}\.\d{2}\.2026, \d{2}:\d{2}$/);
});

test("num groups thousands", () => {
  assert.equal(num(29130).replace(/\s/g, " "), "29 130");
});

test("pct", () => {
  assert.equal(pct(562, 1184), "47.5%");
  assert.equal(pct(1, 0), "—");
});

test("kdaRatio counts zero deaths as one", () => {
  assert.equal(kdaRatio(8, 8, 8), "2.00");
  assert.equal(kdaRatio(3, 0, 1), "4.00");
});

test("names for unknown enum values", () => {
  assert.equal(teamName(1), "Сапфир");
  assert.equal(teamName(16), "Команда 16");
  assert.equal(gameModeName(4), "Street Brawl");
  assert.equal(gameModeName(99), "Режим 99");
});
