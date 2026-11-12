// Table specs for the pages: API data -> localized, formatted rows.
// Runs on the server; the result is plain data for components/DataTable.tsx.
import type { Death, HeroStats, Match, MatchPlayer } from "../api/schema.gen.ts";
import type { I18n } from "../i18n/core.ts";
import type { BuildEntry } from "./build.ts";
import { duration, kda, kdaRatio } from "./format.ts";
import type { Lookup } from "./gamedata.ts";
import { heroRows } from "./heroes.ts";
import { tileOf } from "./icons.ts";
import type { TableSpec } from "./tableSpec.ts";

type Ctx = { i18n: I18n; look: Lookup };

export function heroTable({ i18n, look }: Ctx, stats: HeroStats[], accountId: string, selectedHero: number): TableSpec {
  const { t } = i18n;
  const roles = [...new Set(stats.map((h) => look.hero(h.heroId)?.type ?? "").filter(Boolean))].sort();
  return {
    label: t("heroes.title"),
    columns: [
      { key: "hero", label: t("col.hero") },
      { key: "matches", label: t("col.matches"), numeric: true },
      { key: "winRate", label: t("col.winRate"), numeric: true },
      { key: "kdaAvg", label: t("col.kdaAvg"), numeric: true },
      { key: "kda", label: t("col.kda"), numeric: true },
      { key: "spm", label: t("col.soulsPerMin"), numeric: true },
      { key: "last", label: t("col.lastPlayed"), numeric: true },
    ],
    rows: heroRows(stats).map((h) => {
      const name = look.heroName(h.heroId);
      return {
        id: String(h.heroId),
        me: h.heroId === selectedHero,
        search: name,
        filters: {
          role: look.hero(h.heroId)?.type ?? "",
          winRate: h.winRate >= 0.5 ? "high" : "low",
        },
        cells: {
          hero: { text: name, href: `/players/${accountId}?hero=${h.heroId}`, icon: look.heroIcon(h.heroId) },
          matches: { text: i18n.num(h.matches), sort: h.matches },
          winRate: {
            text: i18n.pct(h.wins, h.matches),
            sort: h.winRate,
            bar: h.winRate,
            tone: h.winRate >= 0.5 ? "win" : undefined,
          },
          kdaAvg: {
            text: `${i18n.dec(h.avgKills, 1)} / ${i18n.dec(h.avgDeaths, 1)} / ${i18n.dec(h.avgAssists, 1)}`,
            sort: h.avgKills,
          },
          kda: { text: i18n.dec(h.kda, 2), sort: h.kda },
          spm: { text: i18n.num(Math.round(h.soulsPerMin)), sort: h.soulsPerMin },
          last: { text: i18n.date(h.lastPlayed), sort: Number(h.lastPlayed), tone: "muted" },
        },
      };
    }),
    initialSort: { key: "matches", dir: "desc" },
    searchPlaceholder: t("filter.searchHero"),
    filters: [
      {
        key: "role",
        label: t("filter.role"),
        anyLabel: t("filter.all"),
        options: roles.map((r) => ({ value: r, label: i18n.role(r) })),
      },
      {
        key: "winRate",
        label: t("filter.winRate"),
        anyLabel: t("filter.any"),
        options: [
          { value: "high", label: t("filter.winRateHigh") },
          { value: "low", label: t("filter.winRateLow") },
        ],
      },
    ],
    collapseAfter: 10,
  };
}

/** One table per team, winners' and losers' order as in the match. */
export function scoreboardTables({ i18n, look }: Ctx, match: Match, highlightSlot?: number): TableSpec[] {
  const { t } = i18n;
  const teams = [...new Set(match.players.map((p) => p.team))].sort((a, b) => a - b);
  return teams.map((team) => {
    const won = team === match.winningTeam;
    const label = `${i18n.team(team)} · ${won ? t("result.teamWon") : t("result.teamLost")}`;
    return {
      label,
      caption: {
        text: i18n.team(team),
        tone: team === 0 ? "team-0" : team === 1 ? "team-1" : undefined,
        note: won ? t("result.teamWon") : t("result.teamLost"),
      },
      columns: [
        { key: "hero", label: t("col.hero") },
        { key: "kda", label: t("col.kdaShort"), numeric: true },
        { key: "souls", label: t("col.souls"), numeric: true },
        { key: "damage", label: t("col.damage"), numeric: true },
        { key: "healing", label: t("col.healing"), numeric: true },
      ],
      rows: match.players
        .filter((p) => p.team === team)
        .map((p: MatchPlayer) => {
          const name = look.heroName(p.heroId);
          return {
            id: String(p.slot),
            me: p.slot === highlightSlot,
            search: name,
            cells: {
              hero: {
                text: name,
                href: `/matches/${match.matchId}/players/${p.accountId}`,
                icon: look.heroIcon(p.heroId),
                tag: p.abandonTimeS > 0 ? t("match.left") : undefined,
                tagTitle: p.abandonTimeS > 0 ? t("match.leftTitle") : undefined,
              },
              kda: { text: kda(p.kills, p.deaths, p.assists), sort: kdaRatio(p.kills, p.deaths, p.assists) },
              souls: { text: i18n.num(p.netWorth), sort: p.netWorth },
              damage: { text: i18n.num(p.playerDamage), sort: p.playerDamage },
              healing: { text: i18n.num(p.playerHealing), sort: p.playerHealing },
            },
          };
        }),
      initialSort: { key: "souls", dir: "desc" },
      searchPlaceholder: t("filter.searchHero"),
    };
  });
}

export function deathsTable({ i18n, look }: Ctx, deaths: Death[], players: MatchPlayer[]): TableSpec {
  const { t } = i18n;
  const killerHero = (slot: number) => players.find((x) => x.slot === slot)?.heroId;
  const killer = (slot: number) => {
    const id = killerHero(slot);
    return id === undefined ? "—" : look.heroName(id);
  };
  const killers = [...new Set(deaths.map((d) => killer(d.killerSlot)))].sort((a, b) => a.localeCompare(b, i18n.locale));
  return {
    label: t("match.deaths"),
    columns: [
      { key: "t", label: t("col.time"), numeric: true },
      { key: "killer", label: t("col.killer") },
      { key: "ttk", label: t("col.timeToKill"), numeric: true },
      { key: "respawn", label: t("col.respawn"), numeric: true },
    ],
    rows: deaths.map((d, i) => ({
      id: `${d.t}-${i}`,
      search: killer(d.killerSlot),
      filters: { killer: killer(d.killerSlot) },
      cells: {
        t: { text: duration(d.t), sort: d.t },
        killer: {
          text: killer(d.killerSlot),
          icon: killerHero(d.killerSlot) === undefined ? undefined : look.heroIcon(killerHero(d.killerSlot)!),
        },
        // Negative means the game did not record it.
        ttk: {
          text: d.timeToKillS >= 0 ? t("unit.seconds", { n: i18n.dec(d.timeToKillS, 1) }) : "—",
          sort: d.timeToKillS,
        },
        respawn: { text: t("unit.seconds", { n: d.respawnS }), sort: d.respawnS },
      },
    })),
    initialSort: { key: "t", dir: "asc" },
    filters: [
      {
        key: "killer",
        label: t("filter.killer"),
        anyLabel: t("filter.any"),
        options: killers.map((k) => ({ value: k, label: k })),
      },
    ],
    collapseAfter: 10,
  };
}

export function buildTable({ i18n, look }: Ctx, entries: BuildEntry[]): TableSpec {
  const { t } = i18n;
  const typeText = (e: BuildEntry) =>
    e.kind === "item"
      ? e.tier
        ? t("build.itemTier", { tier: e.tier })
        : t("build.item")
      : e.kind === "unlock"
        ? t("build.unlock")
        : t("build.upgrade", { n: e.level });
  const slots = (["weapon", "vitality", "spirit"] as const).filter((s) => entries.some((e) => e.slot === s));
  return {
    label: t("match.build"),
    columns: [
      { key: "t", label: t("col.time"), numeric: true },
      { key: "what", label: t("col.what") },
      { key: "type", label: t("col.type") },
      { key: "sold", label: t("col.sold"), numeric: true },
    ],
    rows: entries.map((e, i) => ({
      id: `${e.t}-${e.itemId}-${i}`,
      search: e.name,
      filters: { kind: e.kind === "item" ? "items" : "abilities", slot: e.slot },
      cells: {
        t: { text: duration(e.t), sort: e.t },
        what: {
          text: e.name,
          strike: e.soldT > 0,
          icon: look.abilityIcon(e.itemId),
          iconTile: tileOf(e.slot, e.kind === "item"),
        },
        type: { text: typeText(e), tone: "muted" },
        sold: { text: e.soldT ? duration(e.soldT) : "", sort: e.soldT, tone: "muted" },
      },
    })),
    initialSort: { key: "t", dir: "asc" },
    searchPlaceholder: t("filter.search"),
    filters: [
      {
        key: "kind",
        label: t("filter.kind"),
        anyLabel: t("filter.all"),
        options: [
          { value: "items", label: t("build.items") },
          { value: "abilities", label: t("build.abilities") },
        ],
      },
      {
        key: "slot",
        label: t("filter.slot"),
        anyLabel: t("filter.any"),
        options: slots.map((s) => ({ value: s, label: t(`slot.${s}`) })),
      },
    ],
    collapseAfter: 12,
  };
}
