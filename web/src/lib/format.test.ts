import assert from "node:assert/strict";
import { test } from "node:test";

import { duration, kda, kdaRatio } from "./format.ts";

test("duration", () => {
  assert.equal(duration(0), "0:00");
  assert.equal(duration(75), "1:15");
  assert.equal(duration(1728), "28:48");
  assert.equal(duration(3725), "1:02:05");
  assert.equal(duration(-5), "0:00");
});

test("kda", () => {
  assert.equal(kda(8, 8, 8), "8 / 8 / 8");
});

test("kdaRatio counts zero deaths as one", () => {
  assert.equal(kdaRatio(8, 8, 8), 2);
  assert.equal(kdaRatio(3, 0, 1), 4);
});
