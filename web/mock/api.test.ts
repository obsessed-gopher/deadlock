import assert from "node:assert/strict";
import { join } from "node:path";
import { test } from "node:test";

import type {
  ListPlayerHeroStatsResponse,
  ListPlayerMatchesResponse,
  Match,
  Player,
  PlayerMatch,
  RpcStatus,
} from "../src/api/schema.gen.ts";
import { createMockApi, loadFixtures, normalizeAccountId } from "./api.ts";

// Runs against the committed fixtures: they are part of the contract.
const fx = loadFixtures(join(import.meta.dirname, "fixtures"));
const ACCOUNT = String(fx.accountId);
const STEAM_ID64 = String(76561197960265728n + BigInt(fx.accountId));

function call<T>(method: string, url: string, now?: () => number) {
  const r = createMockApi(fx, now).handle(method, url);
  return { status: r.status, body: r.body as T };
}

test("normalizeAccountId matches the Go rules", () => {
  assert.equal(normalizeAccountId("1517505834"), 1517505834);
  assert.equal(normalizeAccountId("76561199477771562"), 1517505834);
  for (const bad of ["0", "", "abc", "-1", "4294967296", "76561197960265728", "99999999999999999999"]) {
    assert.equal(normalizeAccountId(bad), null, bad);
  }
});

test("player counters are consistent with the fixture rows", () => {
  const { status, body } = call<Player>("GET", `/v1/players/${STEAM_ID64}`);
  assert.equal(status, 200);
  assert.equal(body.accountId, fx.accountId);
  assert.equal(body.matchesTotal, fx.rows.length);
  assert.equal(body.matchesWithMeta + body.matchesPending <= body.matchesTotal, true);
  assert.equal(body.matchesWithMeta > 0, true);
});

test("every OK row links to a breakdown the mock can serve", () => {
  const { body } = call<ListPlayerMatchesResponse>("GET", `/v1/players/${ACCOUNT}/matches?pageSize=200`);
  const ok = body.matches.filter((m) => m.metaStatus === "META_STATUS_OK");
  assert.equal(ok.length > 0, true);
  for (const m of ok) {
    assert.equal(call("GET", `/v1/matches/${m.matchId}/players/${ACCOUNT}`).status, 200, m.matchId);
  }
});

test("pagination is keyset by match id and ends with an empty token", () => {
  const seen: string[] = [];
  let token = "";
  for (let i = 0; i < 10; i++) {
    const q = `pageSize=50${token ? `&pageToken=${token}` : ""}`;
    const { body } = call<ListPlayerMatchesResponse>("GET", `/v1/players/${ACCOUNT}/matches?${q}`);
    seen.push(...body.matches.map((m) => m.matchId));
    token = body.nextPageToken;
    if (!token) break;
  }
  assert.equal(seen.length, fx.rows.length);
  assert.equal(new Set(seen).size, seen.length);
  const sorted = [...seen].sort((a, b) => (BigInt(b) > BigInt(a) ? 1 : -1));
  assert.deepEqual(seen, sorted);
});

test("every sort pages through all rows in order, ties broken by match id", () => {
  const sorts = ["MATCH_SORT_START_TIME", "MATCH_SORT_KILLS", "MATCH_SORT_NET_WORTH", "MATCH_SORT_DURATION"];
  for (const sort of sorts) {
    for (const ascending of [false, true]) {
      const seen: ListPlayerMatchesResponse["matches"] = [];
      let token = "";
      for (let i = 0; i < 20; i++) {
        const q = new URLSearchParams({ pageSize: "17", sort, ascending: String(ascending) });
        if (token) q.set("pageToken", token);
        const { status, body } = call<ListPlayerMatchesResponse>("GET", `/v1/players/${ACCOUNT}/matches?${q}`);
        assert.equal(status, 200);
        assert.equal(body.total, fx.rows.length);
        seen.push(...body.matches);
        token = body.nextPageToken;
        if (!token) break;
      }
      assert.equal(seen.length, fx.rows.length, `${sort} asc=${ascending}`);
      assert.equal(new Set(seen.map((m) => m.matchId)).size, seen.length);
    }
  }
  const { body } = call<ListPlayerMatchesResponse>("GET", `/v1/players/${ACCOUNT}/matches?pageSize=200&sort=MATCH_SORT_KILLS`);
  const kills = body.matches.map((m) => m.kills);
  assert.deepEqual(kills, [...kills].sort((a, b) => b - a));
});

test("filters: result, game mode, metadata; total counts all pages", () => {
  const list = (q: string) =>
    call<ListPlayerMatchesResponse>("GET", `/v1/players/${ACCOUNT}/matches?pageSize=5&${q}`).body;
  const wins = list("result=MATCH_RESULT_WIN");
  assert.equal(wins.total, fx.rows.filter((r) => r.won).length);
  assert.ok(wins.matches.every((m) => m.won));
  const losses = list("result=MATCH_RESULT_LOSS");
  assert.equal(wins.total + losses.total, fx.rows.length);
  const brawl = list("gameMode=4&pageSize=200");
  assert.ok(brawl.matches.every((m) => m.gameMode === 4));
  const ok = list("metaStatus=META_STATUS_OK&pageSize=200");
  assert.ok(ok.total > 0 && ok.matches.every((m) => m.metaStatus === "META_STATUS_OK"));
  for (const bad of ["sort=MATCH_SORT_X", "result=DRAW", "metaStatus=X", "pageToken=101"]) {
    assert.equal(call("GET", `/v1/players/${ACCOUNT}/matches?${bad}`).status, 400, bad);
  }
});

test("error triggers behave like the real gateway", () => {
  const internal = call<RpcStatus>("GET", "/v1/matches/500");
  assert.deepEqual([internal.status, internal.body.code], [500, 13]);
  assert.equal(call("GET", `/v1/matches/500/players/${ACCOUNT}`).status, 500);
  const down = call<RpcStatus>("GET", "/v1/players/503");
  assert.deepEqual([down.status, down.body.code], [503, 14]);
  assert.equal(call("POST", "/v1/players/503:sync").status, 503);
  assert.equal(call("GET", "/v1/players/504").status, 404);
  assert.equal(call("POST", "/v1/players/504:sync").status, 503);
});

test("hero filter and hero stats agree", () => {
  const { body } = call<ListPlayerHeroStatsResponse>("GET", `/v1/players/${ACCOUNT}/heroes`);
  const top = body.heroes[0];
  assert.ok(top);
  assert.equal(body.heroes.reduce((s, h) => s + h.matches, 0), fx.rows.length);
  const list = call<ListPlayerMatchesResponse>("GET", `/v1/players/${ACCOUNT}/matches?pageSize=200&heroId=${top.heroId}`);
  assert.equal(list.body.matches.length, top.matches);
  assert.equal(list.body.matches.every((m) => m.heroId === top.heroId), true);
});

test("match scoreboard comes from a captured breakdown", () => {
  const key = [...fx.playerMatches.keys()][0] ?? "";
  const [mid] = key.split("-");
  const { status, body } = call<Match>("GET", `/v1/matches/${mid}`);
  assert.equal(status, 200);
  assert.equal(body.players.length >= 8, true);
  const pm = call<PlayerMatch>("GET", `/v1/matches/${mid}/players/${body.players[0]?.accountId}`);
  assert.equal(pm.status, 200);
  assert.equal(pm.body.slot, body.players[0]?.slot);
});

test("errors use the gateway format", () => {
  const nf = call<RpcStatus>("GET", "/v1/players/42");
  assert.deepEqual([nf.status, nf.body.code, nf.body.message], [404, 5, "not found"]);
  assert.equal(call<RpcStatus>("GET", "/v1/players/4294967296").status, 400);
  assert.equal(call<RpcStatus>("GET", `/v1/players/${ACCOUNT}/matches?pageToken=garbage`).status, 400);
  assert.equal(call("GET", "/v1/matches/1").status, 404);
  assert.equal(call("GET", "/v2/anything").status, 404);
});

test("sync updates the last sync time of the tracked player only", () => {
  const api = createMockApi(fx, () => 2_000_000_000_000);
  assert.equal(api.handle("POST", `/v1/players/${ACCOUNT}:sync`).status, 200);
  const p = api.handle("GET", `/v1/players/${ACCOUNT}`).body as Player;
  assert.equal(p.lastSyncedAt, "2000000000");
  assert.equal(api.handle("POST", "/v1/players/42:sync").status, 200);
});
