import { join } from "node:path";

import { test as base, expect } from "@playwright/test";

import { createMockApi, loadFixtures } from "../mock/api.ts";
import type { ListPlayerHeroStatsResponse, ListPlayerMatchesResponse } from "../src/api/schema.gen.ts";
import { makeLookup } from "../src/lib/gamedata.ts";

// Expected values come from the same fixtures the mock serves, so tests keep
// passing when fixtures are re-captured.
export const fx = loadFixtures(join(import.meta.dirname, "..", "mock", "fixtures"));
export const look = makeLookup(fx.gameData);
export const ACCOUNT = String(fx.accountId);
export const STEAM_ID64 = String(76561197960265728n + BigInt(fx.accountId));

const mock = createMockApi(fx);
export const rows = (mock.handle("GET", `/v1/players/${ACCOUNT}/matches?pageSize=200`).body as ListPlayerMatchesResponse)
  .matches;
export const heroes = (mock.handle("GET", `/v1/players/${ACCOUNT}/heroes`).body as ListPlayerHeroStatsResponse).heroes;
/** Newest match with a full breakdown. */
export const fullMatch = rows.find((r) => r.metaStatus === "META_STATUS_OK")!;

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

export { expect };
