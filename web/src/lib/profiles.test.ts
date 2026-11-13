import assert from "node:assert/strict";
import { test } from "node:test";

import type { Profile } from "../api/schema.gen.ts";
import { createI18n } from "../i18n/core.ts";
import { NO_PROFILES, avatarOf, playerName, profileMap, uniqueIds } from "./profiles.ts";

const p = (accountId: number, over: Partial<Profile> = {}): Profile => ({
  accountId,
  personaName: `n${accountId}`,
  profileUrl: "",
  avatar: "s.jpg",
  avatarMedium: "m.jpg",
  avatarFull: "f.jpg",
  country: "",
  ...over,
});

test("names fall back to the localized player title", () => {
  const ru = createI18n({ locale: "ru", names: "en" });
  const ps = profileMap([p(1), p(2, { personaName: "  " })]);
  assert.equal(playerName(ru, 1, ps), "n1");
  assert.equal(playerName(ru, "2", ps), "Игрок 2");
  assert.equal(playerName(ru, 3, NO_PROFILES), "Игрок 3");
});

test("avatars by size, falling back to the small one", () => {
  const ps = profileMap([p(1), p(2, { avatarMedium: "", avatarFull: "" })]);
  assert.equal(avatarOf(1, ps), "m.jpg");
  assert.equal(avatarOf(1, ps, "full"), "f.jpg");
  assert.equal(avatarOf(2, ps, "full"), "s.jpg");
  assert.equal(avatarOf(3, ps), "");
});

test("uniqueIds keeps order and the limit", () => {
  assert.deepEqual(uniqueIds([3, "1", 3, 2, "1"]), ["3", "1", "2"]);
  assert.deepEqual(uniqueIds([1, 2, 3], 2), ["1", "2"]);
});
