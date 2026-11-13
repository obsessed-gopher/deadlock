// Main flows in the default setup of a Russian browser: Russian interface,
// English hero and item names. Other languages: i18n.spec.ts.
import {
  ACCOUNT,
  STEAM_ID64,
  column,
  expect,
  expectNoHorizontalScroll,
  fullMatch,
  heroes,
  look,
  parseNum,
  ru,
  rows,
  test,
} from "./helpers.ts";
import { DOWN_VANITY, KNOWN_VANITY } from "../mock/api.ts";

const { t } = ru;

test("search by SteamID64 opens the canonical profile", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel(t("home.inputLabel")).fill(STEAM_ID64);
  await page.getByRole("button", { name: t("home.open") }).click();
  await expect(page).toHaveURL(`/players/${ACCOUNT}`);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(t("player.title", { id: ACCOUNT }));
});

test("a bad id is explained on the search form, not an error page", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel(t("home.inputLabel")).fill("abc");
  await page.getByRole("button", { name: t("home.open") }).click();
  await expect(page).toHaveURL("/?error=badId");
  // Next.js has its own (empty) role=alert route announcer: match by text.
  await expect(page.getByRole("alert").filter({ hasText: t("error.badId") })).toBeVisible();
});

test("search by a Steam profile link, numeric or custom", async ({ page }) => {
  for (const link of [
    `https://steamcommunity.com/profiles/${STEAM_ID64}/`,
    `steamcommunity.com/id/${KNOWN_VANITY}`,
    `[U:1:${ACCOUNT}]`,
  ]) {
    await page.goto("/");
    await page.getByLabel(t("home.inputLabel")).fill(link);
    await page.getByRole("button", { name: t("home.open") }).click();
    await expect(page).toHaveURL(`/players/${ACCOUNT}`);
  }
});

test("a custom link Steam does not know, or a Steam outage, is explained on the form", async ({ page }) => {
  for (const [name, error] of [
    ["nobody-here", "steamNotFound"],
    [DOWN_VANITY, "steamDown"],
  ] as const) {
    await page.goto("/");
    await page.getByLabel(t("home.inputLabel")).fill(`https://steamcommunity.com/id/${name}`);
    await page.getByRole("button", { name: t("home.open") }).click();
    await expect(page).toHaveURL(`/?error=${error}`);
    await expect(page.getByRole("alert").filter({ hasText: t(`error.${error}`) })).toBeVisible();
  }
});

test("home lists tracked accounts", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("link", { name: ACCOUNT }).click();
  await expect(page).toHaveURL(`/players/${ACCOUNT}`);
});

test("profile: totals, hero table collapsed to 10 with English names", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}/heroes`);
  const wins = rows.filter((r) => r.won).length;
  await expect(page.getByText(t("player.summary", { matches: rows.length, winRate: ru.pct(wins, rows.length) }))).toBeVisible();
  await expect(page.getByRole("link", { name: t("tab.heroes") })).toHaveAttribute("aria-current", "page");
  const table = page.getByRole("table", { name: t("heroes.title"), exact: true });
  await expect(table.locator("tbody tr")).toHaveCount(Math.min(10, heroes.length));
  await expect(table.locator("tbody tr").first()).toContainText(look.heroName(heroes[0]!.heroId));
  if (heroes.length > 10) {
    await page.getByRole("button", { name: t("table.showAll", { n: heroes.length }) }).click();
    await expect(table.locator("tbody tr")).toHaveCount(heroes.length);
    await page.getByRole("button", { name: t("table.collapse") }).click();
    await expect(table.locator("tbody tr")).toHaveCount(10);
  }
  await expectNoHorizontalScroll(page);
});

test("hero table: sort by any column, both directions", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}/heroes`);
  const name = t("heroes.title");
  const header = page.getByRole("table", { name, exact: true }).getByRole("columnheader", { name: t("col.kda") });
  await header.getByRole("button").click();
  await expect(header).toHaveAttribute("aria-sort", "descending");
  await page.getByRole("button", { name: t("table.showAll", { n: heroes.length }) }).click();
  const desc = (await column(page, name, 5)).map(parseNum);
  expect(desc).toEqual([...desc].sort((a, b) => b - a));
  await header.getByRole("button").click();
  await expect(header).toHaveAttribute("aria-sort", "ascending");
  const asc = (await column(page, name, 5)).map(parseNum);
  expect(asc).toEqual([...asc].sort((a, b) => a - b));
  // Text column: alphabetical first.
  const heroHeader = page.getByRole("table", { name, exact: true }).getByRole("columnheader", { name: t("col.hero") });
  await heroHeader.getByRole("button").click();
  const names = await column(page, name, 1);
  expect(names).toEqual([...names].sort((a, b) => a.localeCompare(b, "ru", { sensitivity: "base" })));
});

test("hero table: search and filters", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}/heroes`);
  const name = t("heroes.title");
  const top = look.heroName(heroes[0]!.heroId);
  await page.getByLabel(`${name}: ${t("filter.searchHero")}`).fill(top.slice(0, 3).toLowerCase());
  await expect(page.getByRole("table", { name, exact: true }).locator("tbody tr").first()).toContainText(top);
  await page.getByLabel(`${name}: ${t("filter.searchHero")}`).fill("zzzz");
  await expect(page.getByRole("cell", { name: t("table.noRows") })).toBeVisible();
  await page.getByLabel(`${name}: ${t("filter.searchHero")}`).fill("");

  const role = look.hero(heroes[0]!.heroId)!.type;
  const expected = heroes.filter((h) => look.hero(h.heroId)?.type === role).length;
  await page.getByLabel(`${name}: ${t("filter.role")}`).selectOption(role);
  if (expected > 10) await page.getByRole("button", { name: t("table.showAll", { n: expected }) }).click();
  await expect(page.getByRole("table", { name, exact: true }).locator("tbody tr")).toHaveCount(expected);
});

test("matches: server-side sort by souls, then reversed", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}/matches`);
  const name = t("matches.title");
  await page.getByRole("link", { name: t("col.souls") }).click();
  await expect(page).toHaveURL(`/players/${ACCOUNT}/matches?sort=souls&dir=desc`);
  const desc = (await column(page, name, 5)).map(parseNum);
  expect(desc[0]).toBe(Math.max(...rows.map((r) => r.netWorth)));
  expect(desc).toEqual([...desc].sort((a, b) => b - a));
  await page.getByRole("link", { name: t("col.souls") }).click();
  await expect(page).toHaveURL(`/players/${ACCOUNT}/matches?sort=souls&dir=asc`);
  const asc = (await column(page, name, 5)).map(parseNum);
  expect(asc[0]).toBe(Math.min(...rows.map((r) => r.netWorth)));
});

test("matches: result, hero and reset filters", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}/matches`);
  const name = t("matches.title");
  await page.getByRole("navigation", { name: t("filter.result") }).getByRole("link", { name: t("result.wins") }).click();
  await expect(page).toHaveURL(`/players/${ACCOUNT}/matches?result=win`);
  const wins = rows.filter((r) => r.won).length;
  await expect(page.getByRole("heading", { name: new RegExp(t("matches.title")) })).toContainText(String(wins));
  for (const text of await column(page, name, 3)) expect(text).toBe(t("result.win"));

  const hero = rows.find((r) => r.won)!.heroId;
  await page.getByLabel(t("filter.hero")).selectOption(String(hero));
  await page.getByRole("button", { name: t("filter.apply") }).click();
  await expect(page).toHaveURL(new RegExp(`hero=${hero}.*result=win|result=win.*hero=${hero}`));
  const heroWins = rows.filter((r) => r.won && r.heroId === hero).length;
  await expect(page.getByRole("table", { name, exact: true }).locator("tbody tr")).toHaveCount(heroWins);
  for (const text of await column(page, name, 2)) expect(text).toBe(look.heroName(hero));

  await page.getByRole("link", { name: t("filter.reset") }).click();
  await expect(page).toHaveURL(`/players/${ACCOUNT}/matches`);
});

test("matches: pages show their range and go back to the first", async ({ page }) => {
  test.skip(rows.length <= 50, "fixtures fit on one page");
  await page.goto(`/players/${ACCOUNT}/matches`);
  await expect(page.getByText(t("matches.range", { from: 1, to: 50, total: rows.length }))).toBeVisible();
  await page.getByRole("link", { name: t("matches.next") }).click();
  await expect(page.getByText(t("matches.range", { from: 51, to: Math.min(100, rows.length), total: rows.length }))).toBeVisible();
  await page.getByRole("link", { name: t("matches.first") }).click();
  await expect(page).toHaveURL(`/players/${ACCOUNT}/matches`);
});

test("sync button reports the result", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}`);
  await page.getByRole("button", { name: t("player.sync") }).click();
  await expect(page.getByRole("status")).toContainText(t("player.syncDone", { new: 0, queued: "" }).split(",")[0]!);
});

test("match breakdown: chart, sortable scoreboard, build and deaths filters", async ({ page }) => {
  await page.goto(`/players/${ACCOUNT}/matches`);
  await page.getByRole("link", { name: t("matches.breakdown") }).first().click();
  await expect(page).toHaveURL(`/matches/${fullMatch.matchId}/players/${ACCOUNT}`);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(look.heroName(fullMatch.heroId));
  await expect(page.getByRole("img", { name: t("match.chartLabel") }).locator("path")).not.toHaveCount(0);

  // Scoreboard: one table per team, sortable.
  const team = `${ru.team(fullMatch.team)} · ${fullMatch.won ? t("result.teamWon") : t("result.teamLost")}`;
  const dmg = page.getByRole("table", { name: team, exact: true }).getByRole("columnheader", { name: t("col.damage") });
  await dmg.getByRole("button").click();
  const damage = (await column(page, team, 4)).map(parseNum);
  expect(damage).toEqual([...damage].sort((a, b) => b - a));

  // Build: collapsed, filter by kind, search; ability upgrades numbered by order.
  const build = t("match.build");
  await expect(page.getByRole("table", { name: build, exact: true }).locator("tbody tr")).toHaveCount(12);
  await page.getByLabel(`${build}: ${t("filter.kind")}`).selectOption("abilities");
  await page.getByRole("button", { name: /^(Показать все|Свернуть)/ }).last().click();
  for (const type of await column(page, build, 3)) {
    expect([t("build.unlock"), ...[1, 2, 3, 4, 5].map((n) => t("build.upgrade", { n }))]).toContain(type);
  }
  await expect(page.getByRole("cell", { name: t("build.upgrade", { n: 1 }) }).first()).toBeVisible();
  await page.getByLabel(`${build}: ${t("filter.search")}`).fill("zzzz");
  await expect(page.getByRole("table", { name: build, exact: true }).getByRole("cell", { name: t("table.noRows") })).toBeVisible();

  // Deaths: filter by killer.
  const deaths = t("match.deaths");
  const killer = (await column(page, deaths, 2))[0]!;
  await page.getByLabel(`${deaths}: ${t("filter.killer")}`).selectOption(killer);
  for (const k of await column(page, deaths, 2)) expect(k).toBe(killer);
  await expectNoHorizontalScroll(page);
});

test("icons: heroes and items have images, missing ones are placeholders, not 404s", async ({ page, request }) => {
  await page.goto(`/matches/${fullMatch.matchId}/players/${ACCOUNT}`);
  const heroIcon = page.locator("h1").locator("xpath=..").locator("img.icon-portrait");
  await expect(heroIcon).toHaveAttribute("src", `/icons/heroes/${look.hero(fullMatch.heroId)!.key}`);
  // Every icon on the page loaded (the browser would log a failed load).
  const srcs = await page.locator("img").evaluateAll((imgs) =>
    imgs.map((i) => ({ src: i.getAttribute("src"), ok: (i as HTMLImageElement).complete })),
  );
  expect(srcs.length).toBeGreaterThan(20);
  const res = await request.get(srcs[0]!.src!);
  expect(res.status()).toBe(200);
  expect(res.headers()["content-type"]).toBe("image/svg+xml");
  expect((await request.get("/icons/items/x")).status()).toBe(404);
});

test("scoreboard links to another player's breakdown", async ({ page }) => {
  await page.goto(`/matches/${fullMatch.matchId}`);
  const links = page.locator(`table a[href^="/matches/${fullMatch.matchId}/players/"]`);
  await expect(links).not.toHaveCount(0);
  const other = links.filter({ hasNot: page.locator(`[href$="/${ACCOUNT}"]`) }).first();
  const href = await other.getAttribute("href");
  await other.click();
  await expect(page).toHaveURL(href!);
  await expect(page.getByRole("heading", { name: t("match.chart") })).toBeVisible();
});
