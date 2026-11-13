import assert from "node:assert/strict";
import { test } from "node:test";

import { queueOf } from "./queue.ts";

test("ranked is not normal; Street Brawl by game mode; the rest is other", () => {
  assert.equal(queueOf(1, 1), "normal");
  assert.equal(queueOf(1, 8), "normal"); // placement
  assert.equal(queueOf(1, 4), "ranked");
  assert.equal(queueOf(4, 1), "brawl");
  assert.equal(queueOf(4, 4), "brawl");
  assert.equal(queueOf(1, 7), "other"); // hero labs
  assert.equal(queueOf(1, 3), "other"); // bots
  assert.equal(queueOf(0, 0), "other");
});
