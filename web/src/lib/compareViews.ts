// Table spec of the comparison page: matches both players were in.
import type { I18n } from "../i18n/core.ts";
import { kda, kdaRatio } from "./format.ts";
import type { Lookup } from "./gamedata.ts";
import { QUEUES, type QueueKey } from "./queue.ts";
import { winRate } from "./progress.ts";
import type { HeroLine, SharedMatch } from "./seen.ts";
import type { TableSpec } from "./tableSpec.ts";

type Ctx = { i18n: I18n; look: Lookup };

/** The comparison page of A and B, optionally within one queue. */
export function compareHref(a: string | number, b: string | number, queue: QueueKey | "" = ""): string {
  return `/compare/${a}/${b}${queue ? `?queue=${queue}` : ""}`;
}

/** names: display names of A and B (column headings). */
export function sharedTable({ i18n, look }: Ctx, matches: SharedMatch[], names: { a: string; b: string; aId: string }): TableSpec {
  const { t } = i18n;
  const queues = QUEUES.filter((q) => matches.some((m) => m.queue === q));
  return {
    label: t("compare.shared"),
    columns: [
      { key: "date", label: t("col.date") },
      { key: "side", label: t("compare.side") },
      { key: "a", label: names.a },
      { key: "aKda", label: t("col.kdaShort"), numeric: true },
      { key: "b", label: names.b },
      { key: "bKda", label: t("col.kdaShort"), numeric: true },
      { key: "result", label: t("col.result") },
      { key: "queue", label: t("col.mode") },
    ],
    rows: matches.map((m) => ({
      id: m.matchId,
      search: `${look.heroName(m.a.heroId)} ${look.heroName(m.b.heroId)}`,
      filters: { side: m.together ? "together" : "against", queue: m.queue },
      cells: {
        date: {
          text: i18n.date(m.startTime),
          sort: Number(m.startTime),
          href: `/matches/${m.matchId}/players/${names.aId}`,
        },
        side: {
          text: t(m.together ? "compare.sideTogether" : "compare.sideAgainst"),
          tone: m.together ? undefined : "muted",
        },
        a: { text: look.heroName(m.a.heroId), icon: look.heroIcon(m.a.heroId) },
        aKda: { text: kda(m.a.kills, m.a.deaths, m.a.assists), sort: kdaRatio(m.a.kills, m.a.deaths, m.a.assists) },
        b: { text: look.heroName(m.b.heroId), icon: look.heroIcon(m.b.heroId) },
        bKda: { text: kda(m.b.kills, m.b.deaths, m.b.assists), sort: kdaRatio(m.b.kills, m.b.deaths, m.b.assists) },
        result: {
          text: m.a.won ? t("result.win") : t("result.loss"),
          tone: m.a.won ? "win" : "loss",
          badge: true,
        },
        queue: { text: t(`queue.${m.queue}`), tone: "muted" },
      },
    })),
    initialSort: { key: "date", dir: "desc" },
    searchPlaceholder: t("filter.searchHero"),
    filters: [
      {
        key: "side",
        label: t("compare.side"),
        anyLabel: t("filter.all"),
        options: [
          { value: "together", label: t("compare.together") },
          { value: "against", label: t("compare.against") },
        ],
      },
      ...(queues.length > 1
        ? [
            {
              key: "queue",
              label: t("filter.mode"),
              anyLabel: t("filter.all"),
              options: queues.map((q) => ({ value: q, label: t(`queue.${q}`) })),
            },
          ]
        : []),
    ],
    collapseAfter: 10,
  };
}

/** Heroes of a player as seen in stored matches (profiles of untracked players). */
export function seenHeroTable({ i18n, look }: Ctx, lines: HeroLine[]): TableSpec {
  const { t } = i18n;
  return {
    label: t("heroes.title"),
    columns: [
      { key: "hero", label: t("col.hero") },
      { key: "matches", label: t("col.matches"), numeric: true },
      { key: "winRate", label: t("col.winRate"), numeric: true },
      { key: "kda", label: t("col.kda"), numeric: true },
      { key: "spm", label: t("col.soulsPerMin"), numeric: true },
    ],
    rows: lines.map((h) => {
      const name = look.heroName(h.heroId);
      const wr = winRate(h);
      return {
        id: String(h.heroId),
        search: name,
        cells: {
          hero: { text: name, icon: look.heroIcon(h.heroId) },
          matches: { text: i18n.num(h.matches), sort: h.matches },
          winRate: { text: i18n.pct(h.wins, h.matches), sort: wr, bar: wr, tone: wr >= 0.5 ? "win" : undefined },
          kda: { text: i18n.dec(h.kda, 2), sort: h.kda },
          spm: { text: i18n.num(Math.round(h.soulsPerMin)), sort: h.soulsPerMin },
        },
      };
    }),
    initialSort: { key: "matches", dir: "desc" },
    searchPlaceholder: t("filter.searchHero"),
    collapseAfter: 10,
  };
}
