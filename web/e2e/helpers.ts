import { join } from "node:path";

import { type Page, test as base, expect } from "@playwright/test";

import { createMockApi, loadFixtures, mockProfile } from "../mock/api.ts";
import type { ListPlayerHeroStatsResponse, ListPlayerMatchesResponse, PlayerMatch } from "../src/api/schema.gen.ts";
import { createI18n } from "../src/i18n/core.ts";
import { makeLookup } from "../src/lib/gamedata.ts";

// Expected values come from the same fixtures the mock serves, so tests keep
// passing when fixtures are re-captured.
export const fx = loadFixtures(join(import.meta.dirname, "..", "mock", "fixtures"));
export const ACCOUNT = String(fx.accountId);
export const STEAM_ID64 = String(76561197960265728n + BigInt(fx.accountId));

/** Default preferences of a Russian browser: Russian UI, English names. */
export const ru = createI18n({ locale: "ru", names: "en" });
export const en = createI18n({ locale: "en", names: "en" });
export const look = makeLookup(fx.gameData, ru);

/** The name the mock's synthetic Steam profile gives an account, or the fallback. */
export function nameOf(i18n: typeof ru, id: string | number): string {
  return mockProfile(Number(id), fx.accountId)?.personaName ?? i18n.t("player.title", { id: String(id) });
}
export const lookRuNames = makeLookup(fx.gameData, createI18n({ locale: "ru", names: "ru" }));

export const mock = createMockApi(fx);
export const rows = (mock.handle("GET", `/v1/players/${ACCOUNT}/matches?pageSize=200`).body as ListPlayerMatchesResponse)
  .matches;
export const heroes = (mock.handle("GET", `/v1/players/${ACCOUNT}/heroes`).body as ListPlayerHeroStatsResponse).heroes;
/** Newest match with a full breakdown. */
export const fullMatch = rows.find((r) => r.metaStatus === "META_STATUS_OK")!;
export const fullBreakdown = mock.handle("GET", `/v1/matches/${fullMatch.matchId}/players/${ACCOUNT}`)
  .body as PlayerMatch;

/** Numbers as the page shows them ("29 130", "1,087") -> 29130. */
/** Picks an option of our dropdown (components/Select.tsx) by visible names. */
export async function choose(page: Page, label: string, option: string) {
  await page.getByRole("combobox", { name: label, exact: true }).click();
  await page.getByRole("listbox", { name: label, exact: true }).getByRole("option", { name: option, exact: true }).click();
}

export function parseNum(text: string): number {
  return Number(text.replace(/[^\d.,-]/g, "").replace(/[\s,]/g, ""));
}

/** Texts of one column (1-based) of a table found by its accessible name. */
export async function column(page: Page, table: string, n: number): Promise<string[]> {
  return page.getByRole("table", { name: table, exact: true }).locator(`tbody tr td:nth-child(${n})`).allTextContents();
}

/** The page itself must never scroll sideways; wide tables scroll inside their wrapper. */
export async function expectNoHorizontalScroll(page: Page) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  expect(overflow).toBeLessThanOrEqual(0);
}

/**
 * test with a page that fails on any console error or uncaught exception:
 * hydration mismatches and broken client code surface here first.
 */
export const test = base.extend<{ consoleErrors: string[] }>({
  consoleErrors: [
    async ({ page }, use) => {
      const errors: string[] = [];
      page.on("console", (m) => {
        if (m.type() === "error") errors.push(m.text());
      });
      page.on("pageerror", (e) => errors.push(e.message));
      await use(errors);
      expect(errors, "console errors").toEqual([]);
    },
    { auto: true },
  ],
});

/** Drops the browser's own log line for a 404 document we asked for on purpose. */
export function allow404(errors: string[]) {
  const expected = /^Failed to load resource: the server responded with a status of 404/;
  errors.splice(0, errors.length, ...errors.filter((e) => !expected.test(e)));
}

export { expect };
