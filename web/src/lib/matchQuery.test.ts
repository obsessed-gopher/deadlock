import assert from "node:assert/strict";
import { test } from "node:test";

import {
  DEFAULT_STATE,
  filterHref,
  matchApiParams,
  matchStateQuery,
  nextPageHref,
  parseMatchState,
  sortHref,
} from "./matchQuery.ts";

test("empty or garbage query parses to defaults, never an error", () => {
  assert.deepEqual(parseMatchState({}), DEFAULT_STATE);
  assert.deepEqual(
    parseMatchState({ hero: "abc", result: "draw", sort: "x", dir: "up", meta: "?", mode: "-1", page: "101" }),
    DEFAULT_STATE,
  );
  // "from" without a page makes no sense and is dropped.
  assert.equal(parseMatchState({ from: "50" }).from, 0);
});

test("valid query round-trips through the URL", () => {
  const s = parseMatchState({
    hero: "13",
    result: "win",
    mode: "4",
    meta: "ok",
    sort: "kills",
    dir: "asc",
    page: "7_101",
    from: "50",
  });
  assert.deepEqual(s, { hero: 13, result: "win", mode: 4, meta: "ok", sort: "kills", dir: "asc", page: "7_101", from: 50 });
  assert.deepEqual(parseMatchState(Object.fromEntries(new URLSearchParams(matchStateQuery(s)))), s);
  assert.equal(matchStateQuery(DEFAULT_STATE), "");
});

test("sort links: same column flips, new column starts descending, page resets", () => {
  const base = "/players/1";
  const s = { ...DEFAULT_STATE, page: "5_9", from: 50 };
  assert.equal(sortHref(base, s, "date"), "/players/1?sort=date&dir=asc");
  assert.equal(sortHref(base, s, "kills"), "/players/1?sort=kills&dir=desc");
  assert.equal(sortHref(base, { ...s, sort: "kills", dir: "desc" }, "kills"), "/players/1?sort=kills&dir=asc");
});

test("filter links keep other filters and reset the page", () => {
  const s = { ...DEFAULT_STATE, hero: 13, page: "5_9", from: 50 };
  assert.equal(filterHref("/p", s, { result: "loss" }), "/p?hero=13&result=loss");
  assert.equal(filterHref("/p", s, { hero: 0 }), "/p");
});

test("next page carries the token and the offset", () => {
  const s = { ...DEFAULT_STATE, sort: "souls" as const };
  assert.equal(nextPageHref("/p", s, "100_7", 50), "/p?sort=souls&dir=desc&page=100_7&from=50");
});

test("API parameters use the gateway's enum names", () => {
  const s = parseMatchState({ hero: "13", result: "loss", mode: "1", meta: "pending", sort: "souls", dir: "asc", page: "3_4" });
  assert.equal(
    matchApiParams(s, 50).toString(),
    "pageSize=50&heroId=13&result=MATCH_RESULT_LOSS&gameMode=1&metaStatus=META_STATUS_PENDING" +
      "&sort=MATCH_SORT_NET_WORTH&ascending=true&pageToken=3_4",
  );
  assert.equal(matchApiParams(DEFAULT_STATE, 50).toString(), "pageSize=50");
});
