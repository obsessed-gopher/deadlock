import assert from "node:assert/strict";
import { test } from "node:test";

import { createI18n, dictionaries, placeholders, resolvePrefs, translate } from "./core.ts";
import type { MessageKey } from "./en.ts";

test("every locale has every key, non-empty, with the same placeholders", () => {
  const keys = Object.keys(dictionaries.en) as MessageKey[];
  for (const [locale, dict] of Object.entries(dictionaries)) {
    assert.deepEqual(Object.keys(dict).sort(), [...keys].sort(), locale);
    for (const k of keys) {
      assert.ok(dict[k].trim(), `${locale}: ${k} is empty`);
      assert.deepEqual(placeholders(dict[k]), placeholders(dictionaries.en[k]), `${locale}: ${k}`);
    }
  }
});

test("translate fills placeholders and leaves unknown ones visible", () => {
  assert.equal(translate("ru", "player.title", { id: 42 }), "Игрок 42");
  assert.equal(translate("en", "matches.range", { from: 1, to: 50 }), "1–50 of {total}");
});

test("prefs: cookie wins, then Accept-Language, then English; names default to English", () => {
  assert.deepEqual(resolvePrefs("ru", undefined, "en-US"), { locale: "ru", names: "en" });
  assert.deepEqual(resolvePrefs(undefined, undefined, "ru-RU,ru;q=0.9,en;q=0.8"), { locale: "ru", names: "en" });
  assert.deepEqual(resolvePrefs(undefined, undefined, "de-DE,en;q=0.5,ru;q=0.4"), { locale: "en", names: "en" });
  assert.deepEqual(resolvePrefs(undefined, undefined, "en;q=0.3,ru;q=0.9"), { locale: "ru", names: "en" });
  assert.deepEqual(resolvePrefs("fr", "ru", null), { locale: "en", names: "ru" });
  assert.deepEqual(resolvePrefs(undefined, "xx", ""), { locale: "en", names: "en" });
});

test("names follow the names preference, not the interface language", () => {
  const hero = { key: "hero_haze", name: "Haze", nameRu: "Пелена" };
  assert.equal(createI18n({ locale: "ru", names: "en" }).name(hero), "Haze");
  assert.equal(createI18n({ locale: "en", names: "ru" }).name(hero), "Пелена");
  // Missing translation falls back to English, then to the key.
  assert.equal(createI18n({ locale: "ru", names: "ru" }).name({ ...hero, nameRu: "" }), "Haze");
  assert.equal(createI18n({ locale: "en", names: "en" }).name({ ...hero, name: "", nameRu: "" }), "hero_haze");
});

test("locale-aware formatting", () => {
  const ru = createI18n({ locale: "ru", names: "en" });
  const en = createI18n({ locale: "en", names: "en" });
  assert.equal(ru.num(29130).replace(/\s/g, " "), "29 130");
  assert.equal(en.num(29130), "29,130");
  assert.equal(ru.pct(562, 1184).replace(/\s/g, " "), "47,5 %");
  assert.equal(en.pct(562, 1184), "47.5%");
  assert.equal(en.pct(1, 0), "—");
  assert.equal(ru.dec(2, 2), "2,00");
  assert.equal(en.date("0"), "—");
  assert.match(ru.date("1794330963"), /^\d{2}\.\d{2}\.2026, \d{2}:\d{2}$/);
});

test("enum names, with a fallback for values the game adds later", () => {
  const ru = createI18n({ locale: "ru", names: "en" });
  const en = createI18n({ locale: "en", names: "en" });
  assert.equal(ru.team(1), "Сапфир");
  assert.equal(en.team(16), "Team 16");
  assert.equal(en.gameMode(4), "Street Brawl");
  assert.equal(ru.gameMode(99), "Режим 99");
  assert.equal(en.matchMode(4), "Ranked");
  assert.equal(ru.matchMode(5), "Тип 5");
  assert.equal(ru.role("marksman"), "Стрелок");
  assert.equal(en.role("support"), "support");
});
