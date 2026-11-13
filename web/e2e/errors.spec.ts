// Errors are localized pages, never a raw 500. The mock fails on purpose for
// match 500 (Internal) and player 503 (Unavailable); see mock/api.ts.
import { ACCOUNT, allow404, en, expect, ru, test } from "./helpers.ts";

const { t } = ru;

test("API internal error: localized page with retry, HTTP 200", async ({ page }) => {
  const res = await page.goto("/matches/500");
  expect(res?.status()).toBe(200);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(t("error.internal.title"));
  await expect(page.getByText(t("error.internal.text"))).toBeVisible();
  await expect(page.getByRole("link", { name: t("error.retry") })).toHaveAttribute("href", "/matches/500");
  await page.goto(`/matches/500/players/${ACCOUNT}`);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(t("error.internal.title"));
});

test("API unavailable: localized page", async ({ page }) => {
  const res = await page.goto("/players/503");
  expect(res?.status()).toBe(200);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(t("error.unavailable.title"));
  await expect(page.getByRole("link", { name: t("error.home") })).toBeVisible();
});

test.describe("English", () => {
  test.use({ locale: "en-US" });

  test("errors in English too", async ({ page, consoleErrors }) => {
    await page.goto("/players/503");
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(en.t("error.unavailable.title"));
    const res = await page.goto("/players/abc");
    expect(res?.status()).toBe(404);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(en.t("error.notFound.title"));
    allow404(consoleErrors);
  });
});

test("unknown player offers a sync; bad ids and unknown matches are 404", async ({ page, consoleErrors }) => {
  await page.goto("/players/42");
  await expect(page.getByText(t("player.noData"))).toBeVisible();
  await expect(page.getByRole("button", { name: t("player.sync") })).toBeVisible();

  for (const url of ["/players/abc", "/matches/1", "/matches/1/players/42", "/nope"]) {
    const res = await page.goto(url);
    expect(res?.status(), url).toBe(404);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(t("error.notFound.title"));
  }
  allow404(consoleErrors);
});

test("garbage in the match list URL falls back to defaults", async ({ page }) => {
  const res = await page.goto(`/players/${ACCOUNT}/matches?sort=bogus&dir=up&result=draw&page=xyz&hero=abc`);
  expect(res?.status()).toBe(200);
  await expect(page.getByRole("table", { name: t("matches.title"), exact: true })).toBeVisible();
});

test("failed sync is reported in place, not as an error page", async ({ page }) => {
  // Player 504: unknown to the mock, and its sync fails with Unavailable.
  await page.goto("/players/504");
  await page.getByRole("button", { name: t("player.sync") }).click();
  await expect(page.getByRole("status")).toHaveText(
    t("error.syncFailed", { reason: t("error.unavailable.text") }),
  );
});
