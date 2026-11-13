// Profiles of untracked players, the "me vs them" comparison and queues on
// the progress tab. Expected values come from the fixtures the mock serves.
import { countQueues } from "../src/lib/queue.ts";
import { shared } from "../src/lib/seen.ts";
import type { SeenMatch } from "../src/api/schema.gen.ts";
import { ACCOUNT, STEAM_ID64, choose, expect, expectNoHorizontalScroll, fx, mock, nameOf, ru, test } from "./helpers.ts";

const { t } = ru;
const seen = (id: string | number) => (mock.handle("GET", `/v1/players/${id}/seen`).body as { matches: SeenMatch[] }).matches;
// Someone from a fully stored match, not tracked by the mock.
const other = String([...fx.playerMatches.values()][0]!.match!.players.find((p) => p.accountId !== fx.accountId)!.accountId);

test("an untracked player from a match has a profile with what we know", async ({ page }) => {
  await page.goto(`/players/${other}`);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(nameOf(ru, other));
  await expect(page.getByText(t("known.note", { n: ru.num(seen(other).length) }))).toBeVisible();
  await expect(page.getByRole("button", { name: t("player.sync") })).toBeVisible();
  await expect(page.getByRole("table", { name: t("heroes.title") })).toBeVisible();
  await page.getByRole("link", { name: t("compare.withMe") }).click();
  await expect(page).toHaveURL(`/compare/${ACCOUNT}/${other}`);
  await expectNoHorizontalScroll(page);
});

test("comparison: shared matches, metrics, heroes; SteamID64 redirects", async ({ page }) => {
  await page.goto(`/compare/${STEAM_ID64}/${other}`);
  await expect(page).toHaveURL(`/compare/${ACCOUNT}/${other}`);
  const both = shared(seen(ACCOUNT), seen(other));
  await expect(page.getByRole("heading", { name: new RegExp(`^${t("compare.shared")}`) })).toBeVisible();
  await expect(page.getByRole("table", { name: t("compare.metrics") })).toBeVisible();
  const table = page.getByRole("table", { name: t("compare.shared"), exact: true });
  await expect(table.locator("tbody tr")).toHaveCount(both.matches.length);
  await choose(page, `${t("compare.shared")}: ${t("compare.side")}`, t("compare.against"));
  await expect(table.locator("tbody tr")).toHaveCount(Math.max(1, both.against.matches));
  await expectNoHorizontalScroll(page);
});

test("comparison by queue keeps the queue in the URL", async ({ page }) => {
  await page.goto(`/compare/${ACCOUNT}/${other}`);
  const nav = page.getByRole("navigation", { name: t("filter.mode") });
  await nav.getByRole("link", { name: new RegExp(`^${t("queue.normal")}`) }).click();
  await expect(page).toHaveURL(`/compare/${ACCOUNT}/${other}?queue=normal`);
  await expect(nav.getByRole("link", { name: new RegExp(`^${t("queue.normal")}`) })).toHaveAttribute("aria-current", "true");
});

test("friend cards and other tracked-player shortcuts lead to the comparison", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}/peers`);
  const card = page.getByTestId("friend").first();
  test.skip((await card.count()) === 0, "no friends in the fixtures");
  await card.getByRole("link", { name: t("compare.link") }).click();
  await expect(page).toHaveURL(new RegExp(`/compare/${ACCOUNT}/\\d+$`));
  // The owner's own profile has no "compare with me".
  await page.goto(`/players/${ACCOUNT}`);
  await expect(page.getByRole("link", { name: t("compare.withMe") })).toHaveCount(0);
});

test("progress by queue: counts per queue, the choice in the URL", async ({ page }) => {
  const counts = countQueues(fx.facts.matches);
  test.skip(counts.brawl === 0, "no Street Brawl matches in the fixtures");
  await page.goto(`/players/${ACCOUNT}?trend=kda`);
  const nav = page.getByRole("navigation", { name: t("filter.mode") });
  await expect(nav.getByRole("link", { name: `${t("queue.brawl")} ${ru.num(counts.brawl)}` })).toBeVisible();
  await nav.getByRole("link", { name: new RegExp(`^${t("queue.brawl")}`) }).click();
  await expect(page).toHaveURL(`/players/${ACCOUNT}?trend=kda&queue=brawl`);
  await expect(page.getByRole("heading", { level: 2, name: t("form.title") })).toBeVisible();
  await nav.getByRole("link", { name: new RegExp(`^${t("filter.all")}`) }).click();
  await expect(page).toHaveURL(`/players/${ACCOUNT}?trend=kda`);
});
