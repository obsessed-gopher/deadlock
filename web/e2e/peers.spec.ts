// Profile "Teammates" tab: party, friends, hero matchups and people lists
// (lib/peers.ts). Expected values come from the fixtures the mock serves.
import { friends, party } from "../src/lib/peers.ts";
import { winRate } from "../src/lib/progress.ts";
import { ACCOUNT, STEAM_ID64, expect, expectNoHorizontalScroll, fx, look, ru, test } from "./helpers.ts";

const { t } = ru;
const p = fx.peers;
const fs = friends(p.allies, p.friendMinMatches);

test("teammates tab has every section", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}/peers`);
  await expect(page.getByRole("navigation", { name: t("tab.label") }).getByRole("link", { name: t("tab.peers") })).toHaveAttribute(
    "aria-current",
    "page",
  );
  for (const h of ["party.title", "friends.title", "matchups.title", "allies.title", "enemies.title"] as const) {
    await expect(page.getByRole("heading", { level: 2, name: new RegExp(`^${t(h)}`) })).toBeVisible();
  }
  await expectNoHorizontalScroll(page);
});

test("party compares solo and with friends", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}/peers`);
  const { solo, party: withF } = party(p.partySizes);
  const section = page.getByRole("region", { name: t("party.title") });
  await expect(section.getByText(ru.pct(winRate(solo), 1), { exact: true }).first()).toBeVisible();
  await expect(section.getByText(ru.pct(winRate(withF), 1), { exact: true }).first()).toBeVisible();
  await expect(section.getByRole("img", { name: t("party.chartLabel") })).toBeVisible();
});

test("a friend card opens the friend's profile", async ({ page }) => {
  test.skip(fs.length === 0, "no friends in the fixtures");
  await page.goto(`/players/${ACCOUNT}/peers`);
  await expect(page.getByTestId("friend")).toHaveCount(fs.length);
  const first = page.getByTestId("friend").first();
  await expect(first).toContainText(look.heroName(fs[0]!.topHeroId));
  await expect(first.getByRole("link", { name: t("friends.last", { date: "" }).trim(), exact: false })).toHaveAttribute(
    "href",
    `/matches/${fs[0]!.lastMatchId}/players/${ACCOUNT}`,
  );
  await first.getByRole("link", { name: t("player.title", { id: fs[0]!.accountId }) }).click();
  await expect(page).toHaveURL(`/players/${fs[0]!.accountId}`);
});

test("allies table filters friends and searches by id", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}/peers`);
  const table = page.getByRole("table", { name: t("allies.title") });
  await page.getByRole("combobox", { name: `${t("allies.title")}: ${t("filter.who")}` }).selectOption("friends");
  await expect(table.locator("tbody tr")).toHaveCount(fs.length);
  await page.getByRole("combobox", { name: `${t("allies.title")}: ${t("filter.who")}` }).selectOption("");
  const target = p.allies[p.allies.length - 1]!;
  await page.getByRole("searchbox", { name: `${t("allies.title")}: ${t("filter.searchPlayer")}` }).fill(target.accountId);
  await expect(table.getByRole("link", { name: t("player.title", { id: target.accountId }) })).toBeVisible();
});

test("hero matchups list every hero", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}/peers`);
  const table = page.getByRole("table", { name: t("matchups.tableLabel") });
  await expect(table).toBeVisible();
  // Sorted by matches against, most first; any of the tied heroes may lead.
  const max = Math.max(...p.heroes.map((h) => h.againstMatches));
  const names = p.heroes.filter((h) => h.againstMatches === max).map((h) => look.heroName(h.heroId));
  await expect(table.locator("tbody tr").first()).toContainText(new RegExp(names.join("|")));
});

test("SteamID64 lands on the same tab; unknown players see no data", async ({ page }) => {
  await page.goto(`/players/${STEAM_ID64}/peers`);
  await expect(page).toHaveURL(`/players/${ACCOUNT}/peers`);
  await page.goto("/players/42/peers");
  await expect(page.getByText(t("player.noData"))).toBeVisible();
});
