import assert from "node:assert/strict";
import { join } from "node:path";
import { test } from "node:test";

import { createMockApi, loadFixtures } from "../../mock/api.ts";
import type { ListPlayerHeroStatsResponse, PlayerMatch } from "../api/schema.gen.ts";
import { createI18n } from "../i18n/core.ts";
import { buildEntries } from "./build.ts";
import { makeLookup } from "./gamedata.ts";
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
  assert.equal(haze?.cells.hero?.href, `/players/${ACC}?hero=13`);
  assert.equal(haze?.me, true);
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
  assert.equal(spec.rows[0]?.cells.type?.text, "ability unlocked");
});
