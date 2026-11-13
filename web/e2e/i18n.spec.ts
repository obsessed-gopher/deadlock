// Languages: interface ru/en (browser preference, header switch, settings)
// and hero/item names (English by default, Russian only from settings).
import { ACCOUNT, en, expect, fullMatch, look, lookRuNames, ru, test } from "./helpers.ts";

test.describe("English browser", () => {
  test.use({ locale: "en-US" });

  test("interface follows Accept-Language", async ({ page }) => {
    await page.goto(`/players/${ACCOUNT}/heroes`);
    await expect(page.locator("html")).toHaveAttribute("lang", "en");
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(en.t("player.title", { id: ACCOUNT }));
    await expect(page.getByRole("heading", { name: new RegExp(en.t("heroes.title")) })).toBeVisible();
  });
});

test("header switch changes the interface language and stays on the page", async ({ page }) => {
  const url = `/players/${ACCOUNT}/matches?sort=souls&dir=desc`;
  await page.goto(url);
  await expect(page.locator("html")).toHaveAttribute("lang", "ru");
  await page.getByRole("button", { name: "EN" }).click();
  await expect(page).toHaveURL(url);
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  await expect(page.getByRole("button", { name: en.t("player.sync") })).toBeVisible();
  await page.getByRole("button", { name: "RU" }).click();
  await expect(page.getByRole("button", { name: ru.t("player.sync") })).toBeVisible();
});

test("names are English by default, Russian after choosing it in settings", async ({ page }) => {
  const hero = fullMatch.heroId;
  await page.goto(`/matches/${fullMatch.matchId}/players/${ACCOUNT}`);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(look.heroName(hero));

  await page.goto("/settings");
  await page.getByRole("radio", { name: "Русский" }).nth(1).check();
  await page.getByRole("button", { name: ru.t("settings.save") }).click();
  await expect(page.getByRole("status")).toHaveText(ru.t("settings.saved"));

  await page.goto(`/matches/${fullMatch.matchId}/players/${ACCOUNT}`);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(lookRuNames.heroName(hero));
  // The interface stays Russian; only the names changed.
  await expect(page.getByRole("heading", { name: ru.t("match.chart") })).toBeVisible();
});

test("settings switch the interface to English while names stay English", async ({ page }) => {
  await page.goto("/settings");
  await page.getByRole("radio", { name: "English" }).first().check();
  await page.getByRole("button", { name: ru.t("settings.save") }).click();
  await expect(page.getByRole("status")).toHaveText(en.t("settings.saved"));
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(en.t("settings.title"));
});
