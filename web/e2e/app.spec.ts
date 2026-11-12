import type { Page } from "@playwright/test";

import { ACCOUNT, STEAM_ID64, expect, fullMatch, heroes, look, rows, test } from "./helpers.ts";

/** The page itself must never scroll sideways; wide tables scroll inside their wrapper. */
async function expectNoHorizontalScroll(page: Page) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  expect(overflow).toBeLessThanOrEqual(0);
}

test("search by SteamID64 opens the canonical profile", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Account id").fill(STEAM_ID64);
  await page.getByRole("button", { name: "Открыть" }).click();
  await expect(page).toHaveURL(`/players/${ACCOUNT}`);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(`Игрок ${ACCOUNT}`);
});

test("home lists tracked accounts", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("link", { name: ACCOUNT }).click();
  await expect(page).toHaveURL(`/players/${ACCOUNT}`);
});

test("profile shows totals, heroes and matches", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}`);
  await expect(page.locator(".stat").filter({ hasText: "Матчей" })).toContainText(String(rows.length));
  const heroTable = page.getByRole("table", { name: "Герои" });
  await expect(heroTable.getByRole("row")).toHaveCount(heroes.length + 1);
  const top = heroes[0]!;
  await expect(heroTable.getByRole("row").nth(1)).toContainText(look.heroName(top.heroId));
  await expect(page.getByRole("table", { name: "Матчи" }).getByRole("row")).toHaveCount(Math.min(50, rows.length) + 1);
  await expectNoHorizontalScroll(page);
});

test("hero filter from the hero table", async ({ page }) => {
  const top = heroes[0]!;
  await page.goto(`/players/${ACCOUNT}`);
  await page.getByRole("table", { name: "Герои" }).getByRole("link", { name: look.heroName(top.heroId) }).click();
  await expect(page).toHaveURL(`/players/${ACCOUNT}?hero=${top.heroId}`);
  const matchRows = page.getByRole("table", { name: "Матчи" }).locator("tbody tr");
  await expect(matchRows).toHaveCount(Math.min(50, top.matches));
  for (const text of await matchRows.locator("td:nth-child(2)").allTextContents()) {
    expect(text).toBe(look.heroName(top.heroId));
  }
});

test("pagination goes to older matches and back", async ({ page }) => {
  test.skip(rows.length <= 50, "fixtures fit on one page");
  await page.goto(`/players/${ACCOUNT}`);
  await page.getByRole("link", { name: "Старше →" }).click();
  await expect(page).toHaveURL(new RegExp(`/players/${ACCOUNT}\\?before=\\d+`));
  await expect(page.getByRole("table", { name: "Матчи" }).locator("tbody tr")).toHaveCount(Math.min(50, rows.length - 50));
  await page.getByRole("link", { name: "← К последним" }).click();
  await expect(page).toHaveURL(`/players/${ACCOUNT}`);
});

test("sync button reports the result", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}`);
  await page.getByRole("button", { name: "Синхронизировать" }).click();
  await expect(page.getByText(/Новых матчей: 0/)).toBeVisible();
});

test("match breakdown: chart, scoreboard, build", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}`);
  await page
    .getByRole("table", { name: "Матчи" })
    .getByRole("row")
    .filter({ has: page.getByRole("link", { name: "разбор →" }) })
    .first()
    .getByRole("link", { name: "разбор →" })
    .click();
  await expect(page).toHaveURL(`/matches/${fullMatch.matchId}/players/${ACCOUNT}`);
  await expect(page.getByRole("heading", { level: 1 })).toContainText(look.heroName(fullMatch.heroId));
  await expect(page.getByRole("heading", { level: 1 })).toContainText(fullMatch.won ? "Победа" : "Поражение");
  await expect(page.getByRole("img", { name: "Души игроков по времени" }).locator("path")).not.toHaveCount(0);
  // Ability upgrades are numbered by order (docs/problems.md, upgrade_id).
  await expect(page.getByRole("cell", { name: "улучшение 1" }).first()).toBeVisible();
  await expectNoHorizontalScroll(page);
});

test("scoreboard links to another player's breakdown", async ({ page }) => {
  await page.goto(`/matches/${fullMatch.matchId}`);
  const links = page.locator(`a[href^="/matches/${fullMatch.matchId}/players/"]`);
  await expect(links).not.toHaveCount(0);
  const other = links.filter({ hasNot: page.locator(`[href$="/${ACCOUNT}"]`) }).first();
  const href = await other.getAttribute("href");
  await other.click();
  await expect(page).toHaveURL(href!);
  await expect(page.getByRole("heading", { name: "Души по времени" })).toBeVisible();
});

test("unknown player offers a sync, bad ids are 404", async ({ page, consoleErrors }) => {
  await page.goto("/players/42");
  await expect(page.getByText("Данных об этом игроке ещё нет")).toBeVisible();
  await expect(page.getByRole("button", { name: "Синхронизировать" })).toBeVisible();

  const res = await page.goto("/players/abc");
  expect(res?.status()).toBe(404);
  await expect(page.getByRole("heading", { name: "Не найдено" })).toBeVisible();
  expect((await page.goto("/matches/1"))?.status()).toBe(404);

  // The browser logs the 404 documents we just asked for; anything else is a real error.
  const expected = /^Failed to load resource: the server responded with a status of 404/;
  const unexpected = consoleErrors.filter((e) => !expected.test(e));
  consoleErrors.splice(0, consoleErrors.length, ...unexpected);
});
