// Profile "Progress" tab: insights computed from match facts (lib/progress.ts).
// Expected values come from the same fixtures the mock serves.
import { MIN_DURATION_S, minHeroMatches, records, speed } from "../src/lib/progress.ts";
import { ACCOUNT, STEAM_ID64, expect, expectNoHorizontalScroll, fx, look, ru, test } from "./helpers.ts";

const { t } = ru;
const facts = fx.facts.matches;

test("progress is the default tab with every section", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}`);
  await expect(page.getByRole("navigation", { name: t("tab.label") }).getByRole("link", { name: t("tab.progress") })).toHaveAttribute(
    "aria-current",
    "page",
  );
  for (const h of ["form.title", "trend.title", "lobby.title", "speed.title", "sessions.title", "records.title"] as const) {
    await expect(page.getByRole("heading", { level: 2, name: t(h) })).toBeVisible();
  }
  await expectNoHorizontalScroll(page);
});

test("form compares the latest matches with the ones before", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}`);
  const n = Math.min(30, Math.floor(facts.filter((f) => f.durationS >= MIN_DURATION_S).length / 2));
  await expect(page.getByText(t("form.sub", { n }))).toBeVisible();
  await expect(page.getByText(new RegExp(`^(${t("form.up")}|${t("form.down")}|${t("form.flat")})$`))).toBeVisible();
});

test("trend metric switch keeps the state in the URL", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}`);
  const nav = page.getByRole("navigation", { name: t("trend.metric") });
  await nav.getByRole("link", { name: t("trend.kda") }).click();
  await expect(page).toHaveURL(`/players/${ACCOUNT}?trend=kda`);
  await expect(nav.getByRole("link", { name: t("trend.kda") })).toHaveAttribute("aria-current", "true");
  await expect(page.getByRole("img", { name: new RegExp(t("col.kda")) })).toBeVisible();
  await nav.getByRole("link", { name: t("trend.spm") }).click();
  await expect(page).toHaveURL(`/players/${ACCOUNT}`);
});

test("development speed shows the souls curve and the fastest heroes", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}`);
  const withMeta = facts.filter((f) => f.hasMeta).length;
  const s = speed(facts, fx.facts.soulMarksS, fx.facts.soulTargets, { minHeroMatches: minHeroMatches(withMeta) });
  const tenMin = s.marks.find((m) => m.t === 600)!;
  await expect(page.locator(".marks")).toContainText(ru.num(Math.round(tenMin.avg)));
  const top = s.heroes[0];
  test.skip(!top, "no hero has enough matches in the fixtures");
  const link = page.locator(".fast-heroes").getByRole("link", { name: look.heroName(top!.heroId) });
  await link.click();
  await expect(page).toHaveURL(`/players/${ACCOUNT}/matches?hero=${top!.heroId}`);
});

test("a record links to the match it was set in", async ({ page }) => {
  const best = records(facts).best[0]!;
  await page.goto(`/players/${ACCOUNT}`);
  await page.locator(".records").getByRole("link", { name: new RegExp(t(`record.${best.key}`)) }).click();
  await expect(page).toHaveURL(`/matches/${best.matchId}/players/${ACCOUNT}`);
});

test("old match list links and SteamID64 land on the right tab", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}?sort=souls&dir=desc`);
  await expect(page).toHaveURL(`/players/${ACCOUNT}/matches?sort=souls&dir=desc`);
  await page.goto(`/players/${STEAM_ID64}/heroes`);
  await expect(page).toHaveURL(`/players/${ACCOUNT}/heroes`);
  await page.goto(`/players/${STEAM_ID64}?trend=deaths`);
  await expect(page).toHaveURL(`/players/${ACCOUNT}?trend=deaths`);
});

test.describe("English browser", () => {
  test.use({ locale: "en-US" });

  test("progress texts are translated", async ({ page }) => {
    await page.goto(`/players/${ACCOUNT}`);
    await expect(page.getByRole("heading", { level: 2, name: "Development speed" })).toBeVisible();
    await expect(page.getByRole("heading", { level: 2, name: "Lobby level" })).toBeVisible();
  });
});
