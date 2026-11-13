import assert from "node:assert/strict";
import { test } from "node:test";

import { parsePlayerRef } from "./playerRef.ts";

const id = (s: string) => ({ kind: "id", id: s });
const vanity = (name: string) => ({ kind: "vanity", name });

test("ids pass through as typed", () => {
  assert.deepEqual(parsePlayerRef(" 1517505834 "), id("1517505834"));
  assert.deepEqual(parsePlayerRef("76561199477771562"), id("76561199477771562"));
  assert.equal(parsePlayerRef("0"), null);
});

test("SteamID2 and SteamID3 become account ids", () => {
  assert.deepEqual(parsePlayerRef("[U:1:1517505834]"), id("1517505834"));
  assert.deepEqual(parsePlayerRef("U:1:1517505834"), id("1517505834"));
  assert.deepEqual(parsePlayerRef("STEAM_0:0:758752917"), id("1517505834"));
  assert.deepEqual(parsePlayerRef("STEAM_1:1:10"), id("21"));
  assert.equal(parsePlayerRef("[U:1:0]"), null);
  assert.equal(parsePlayerRef("[U:1:9999999999]"), null);
});

test("profile links with and without scheme, www, slashes and query", () => {
  for (const s of [
    "https://steamcommunity.com/profiles/76561199477771562",
    "https://steamcommunity.com/profiles/76561199477771562/",
    "http://www.steamcommunity.com/profiles/76561199477771562/games?tab=all",
    "steamcommunity.com/profiles/76561199477771562",
  ]) {
    assert.deepEqual(parsePlayerRef(s), id("76561199477771562"), s);
  }
  assert.deepEqual(parsePlayerRef("https://steamcommunity.com/id/some_name-1/"), vanity("some_name-1"));
  assert.deepEqual(parsePlayerRef("steamcommunity.com/id/Gabe"), vanity("Gabe"));
});

test("anything else is not a player", () => {
  for (const s of [
    "",
    "abc",
    "https://example.com/profiles/76561199477771562",
    "https://steamcommunity.com.evil.io/id/x1",
    "https://steamcommunity.com/profiles/abc",
    "https://steamcommunity.com/id/",
    "https://steamcommunity.com/id/a",
    "https://steamcommunity.com/id/%E2%9C%93ok",
    "https://steamcommunity.com/groups/valve",
  ]) {
    assert.equal(parsePlayerRef(s), null, s);
  }
});
