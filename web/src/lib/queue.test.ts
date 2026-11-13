import assert from "node:assert/strict";
import { test } from "node:test";

import { countQueues, parseQueue, queueOf } from "./queue.ts";

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

test("countQueues and parseQueue", () => {
  assert.deepEqual(
    countQueues([
      { gameMode: 1, matchMode: 1 },
      { gameMode: 1, matchMode: 4 },
      { gameMode: 1, matchMode: 4 },
      { gameMode: 4, matchMode: 1 },
    ]),
    { normal: 1, ranked: 2, brawl: 1, other: 0 },
  );
  assert.equal(parseQueue("ranked"), "ranked");
  assert.equal(parseQueue(["ranked"]), "");
  assert.equal(parseQueue("4"), "");
});
