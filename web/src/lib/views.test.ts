import assert from "node:assert/strict";
import { join } from "node:path";
import { test } from "node:test";

import { OWNER_NAME, createMockApi, loadFixtures } from "../../mock/api.ts";
import type {
  ListPlayerHeroStatsResponse,
  ListPlayerPeersResponse,
  ListProfilesResponse,
  PlayerMatch,
} from "../api/schema.gen.ts";
import { createI18n } from "../i18n/core.ts";
import { buildEntries } from "./build.ts";
import { makeLookup } from "./gamedata.ts";
import { matchupTable, peerTable } from "./peerViews.ts";
import { profileMap } from "./profiles.ts";
import { buildTable, deathsTable, heroTable, scoreboardTables } from "./views.ts";

// Real data: the captured fixtures, match 134969900 (Haze, 8 deaths, a leaver).
const fx = loadFixtures(join(import.meta.dirname, "..", "..", "mock", "fixtures"));
const mock = createMockApi(fx);
const ACC = String(fx.accountId);
const pm = mock.handle("GET", `/v1/matches/134969900/players/${ACC}`).body as PlayerMatch;
const stats = (mock.handle("GET", `/v1/players/${ACC}/heroes`).body as ListPlayerHeroStatsResponse).heroes;

function ctx(locale: "en" | "ru", names: "en" | "ru") {
  const i18n = createI18n({ locale, names });
  return { i18n, look: makeLookup(fx.gameData, i18n) };
}

test("hero table: one row per hero, English names by default even in Russian", () => {
  const spec = heroTable(ctx("ru", "en"), stats, ACC, 13);
  assert.equal(spec.rows.length, stats.length);
  const haze = spec.rows.find((r) => r.id === "13");
  assert.equal(haze?.cells.hero?.text, "Haze");
  assert.equal(haze?.cells.hero?.href, `/players/${ACC}/matches?hero=13`);
  assert.equal(haze?.me, true);
  assert.equal(haze?.cells.hero?.icon, "/icons/heroes/hero_haze");
  assert.equal(spec.columns[0]?.label, "Герой");
  // Every row has a role the role filter offers.
  const roles = new Set(spec.filters?.find((f) => f.key === "role")?.options.map((o) => o.value));
  for (const r of spec.rows) assert.ok(roles.has(r.filters?.role ?? ""), r.id);
});

test("hero table: Russian names when chosen in settings", () => {
  const spec = heroTable(ctx("en", "ru"), stats, ACC, 0);
  assert.equal(spec.rows.find((r) => r.id === "13")?.cells.hero?.text, "Пелена");
  assert.equal(spec.columns[0]?.label, "Hero");
});

test("scoreboard: a table per team, the player marked, the leaver tagged", () => {
  const tables = scoreboardTables(ctx("en", "en"), pm.match!, pm.slot);
  assert.deepEqual(
    tables.map((t) => [t.label, t.rows.length]),
    [
      ["Amber · lost", 6],
      ["Sapphire · won", 6],
    ],
  );
  const amber = tables[0]!.rows;
  assert.equal(amber.filter((r) => r.me).length, 1);
  assert.equal(amber.find((r) => r.me)?.cells.hero?.text, "Haze");
  assert.equal(amber.filter((r) => r.cells.hero?.tag === "left").length, 1);
});

test("scoreboard: players by Steam name with a fallback, linked to their profiles", () => {
  const ids = pm.match!.players.map((p) => p.accountId);
  const profiles = profileMap(
    (mock.handle("GET", `/v1/profiles?${ids.map((id) => `account_ids=${id}`).join("&")}`).body as ListProfilesResponse).profiles,
  );
  const rows = scoreboardTables(ctx("ru", "en"), pm.match!, pm.slot, profiles).flatMap((t) => t.rows);
  const me = rows.find((r) => r.me)!;
  assert.equal(me.cells.player?.text, OWNER_NAME);
  assert.equal(me.cells.player?.href, `/players/${ACC}`);
  for (const p of pm.match!.players) {
    const row = rows.find((r) => r.id === String(p.slot))!;
    const want = profiles.get(String(p.accountId))?.personaName ?? `Игрок ${p.accountId}`;
    assert.equal(row.cells.player?.text, want);
    assert.equal(row.cells.player?.avatar?.name, want);
  }
  // Without profiles every player keeps the fallback.
  const plain = scoreboardTables(ctx("en", "en"), pm.match!, pm.slot).flatMap((t) => t.rows);
  assert.equal(plain.find((r) => r.me)?.cells.player?.text, `Player ${ACC}`);
});

test("deaths: time, killer filter, unknown time-to-kill as a dash", () => {
  const spec = deathsTable(ctx("ru", "en"), pm.deaths, pm.match!.players);
  assert.equal(spec.rows.length, 8);
  assert.equal(spec.rows[0]?.cells.t?.text, "2:45");
  assert.equal(spec.rows[0]?.cells.ttk?.text, "—");
  assert.equal(spec.rows[1]?.cells.ttk?.text, "4,5 с");
  const killers = spec.filters?.[0]?.options.map((o) => o.value) ?? [];
  for (const r of spec.rows) assert.ok(killers.includes(r.filters?.killer ?? ""));
});

test("build: items vs abilities, slots, sold items struck through", () => {
  const i = ctx("en", "en");
  const entries = buildEntries(pm.items, i.look.ability, i.look.abilityName);
  const spec = buildTable(i, entries);
  assert.equal(spec.rows.length, pm.items.length);
  const kinds = new Set(spec.rows.map((r) => r.filters?.kind));
  assert.deepEqual([...kinds].sort(), ["abilities", "items"]);
  const sold = spec.rows.filter((r) => r.cells.what?.strike);
  assert.ok(sold.length > 0);
  assert.ok(sold.every((r) => r.cells.sold?.text));
  assert.equal(spec.rows[0]?.cells.what?.text, "Smoke Bomb");
  // Icons: abilities on a neutral tile, shop items on the tile of their slot.
  assert.equal(spec.rows[0]?.cells.what?.icon, "/icons/abilities/ability_smoke_bomb");
  assert.equal(spec.rows[0]?.cells.what?.iconTile, "ability");
  const item = spec.rows.find((r) => r.filters?.kind === "items" && r.filters.slot === "weapon");
  assert.equal(item?.cells.what?.iconTile, "weapon");
  assert.equal(spec.rows[0]?.cells.type?.text, "ability unlocked");
});

const peers = mock.handle("GET", `/v1/players/${ACC}/peers`).body as ListPlayerPeersResponse;
const total = { matches: peers.matches, wins: peers.wins };

test("peer table: allies link to profiles and matches, friends filter", () => {
  const spec = peerTable(ctx("ru", "en"), {
    label: "Союзники",
    peers: peers.allies,
    total,
    ownerId: ACC,
    friendMin: peers.friendMinMatches,
  });
  assert.equal(spec.rows.length, peers.allies.length);
  const top = spec.rows[0]!;
  const p = peers.allies[0]!;
  assert.equal(top.cells.player?.href, `/players/${p.accountId}`);
  assert.equal(top.cells.last?.href, `/matches/${p.lastMatchId}/players/${ACC}`);
  assert.equal(top.filters?.who, p.matches >= peers.friendMinMatches ? "friends" : "others");
  assert.match(top.cells.delta?.text ?? "", /^([+−]\d+,\d п\.п\.|0,0 п\.п\.|—)$/);
  // Opponents have no friends filter.
  const enemies = peerTable(ctx("en", "en"), { label: "Opponents", peers: peers.enemies, total, ownerId: ACC });
  assert.equal(enemies.filters, undefined);
  assert.equal(enemies.rows[0]?.filters, undefined);
});

test("matchup table: one row per hero, dash for never seen on a side", () => {
  const spec = matchupTable(ctx("en", "en"), peers.heroes, total);
  assert.equal(spec.rows.length, peers.heroes.length);
  const sum = (k: "with" | "against") => spec.rows.reduce((n, r) => n + Number(r.cells[k]?.sort), 0);
  assert.equal(sum("with"), peers.heroes.reduce((n, h) => n + h.withMatches, 0));
  assert.equal(sum("against"), peers.heroes.reduce((n, h) => n + h.againstMatches, 0));
  const none = spec.rows.find((r) => r.cells.with?.sort === 0);
  if (none) assert.equal(none.cells.withWr?.text, "—");
});
