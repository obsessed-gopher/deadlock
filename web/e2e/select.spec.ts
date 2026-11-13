// Our dropdown (components/Select.tsx): mouse, keyboard and outside clicks.
import { ACCOUNT, expect, look, ru, test } from "./helpers.ts";

const { t } = ru;

test("dropdown works with the keyboard and closes on an outside click", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}/matches`);
  const box = page.getByRole("combobox", { name: t("filter.meta") });
  const list = page.getByRole("listbox", { name: t("filter.meta") });
  await expect(box).toHaveText(t("filter.any"));
  await expect(list).toBeHidden();

  await box.focus();
  await page.keyboard.press("ArrowDown");
  await expect(box).toHaveAttribute("aria-expanded", "true");
  await expect(list).toBeVisible();
  await page.keyboard.press("End");
  await expect(list.getByRole("option").last()).toHaveClass(/active/);
  await page.keyboard.press("Enter");
  await expect(list).toBeHidden();
  await expect(box).toHaveText(t("meta.failed"));
  await expect(box).toBeFocused();

  // Escape closes without changing the value.
  await page.keyboard.press("Space");
  await page.keyboard.press("Home");
  await page.keyboard.press("Escape");
  await expect(list).toBeHidden();
  await expect(box).toHaveText(t("meta.failed"));

  await box.click();
  await expect(list).toBeVisible();
  await page.getByRole("heading", { level: 1 }).click();
  await expect(list).toBeHidden();

  // The choice goes into the GET form.
  await page.getByRole("button", { name: t("filter.apply") }).click();
  // Empty choices are not submitted at all.
  await expect(page).toHaveURL(`/players/${ACCOUNT}/matches?meta=failed`);
});

test("hero dropdown: portraits and type-ahead", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}/matches`);
  // Type-ahead by the first letters while closed. Hero names are Latin:
  // Playwright types non-US characters without keydown events.
  const hero = page.getByRole("combobox", { name: t("filter.hero") });
  const target = look.playableHeroes.map((h) => look.heroName(h.id)).sort((a, b) => a.localeCompare(b))[2]!;
  await hero.focus();
  await page.keyboard.type(target.slice(0, 4));
  await expect(hero).toHaveText(target);

  await page.getByRole("combobox", { name: t("filter.hero") }).click();
  const options = page.getByRole("listbox", { name: t("filter.hero") }).getByRole("option");
  expect(await options.count()).toBeGreaterThan(10);
  await expect(options.nth(1).locator("img")).toHaveCount(1);
});
