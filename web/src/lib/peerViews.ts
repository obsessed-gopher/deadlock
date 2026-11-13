// Table specs of the "Teammates" tab: people met and hero matchups.
import type { HeroMatchup, Peer } from "../api/schema.gen.ts";
import type { I18n } from "../i18n/core.ts";
import type { Lookup } from "./gamedata.ts";
import { type Bucket, winRate } from "./progress.ts";
import { MIN_SAMPLE, NOTABLE, withWithout } from "./peers.ts";
import { NO_PROFILES, type Profiles, avatarOf, playerName } from "./profiles.ts";
import type { TableSpec, Tone } from "./tableSpec.ts";

type Ctx = { i18n: I18n; look: Lookup };

/** "3.1 pp" of a share, without a sign. */
export function pp(i18n: I18n, share: number): string {
  return i18n.t("unit.pp", { n: i18n.dec(Math.abs(share) * 100, 1) });
}

/** "+3.1 pp" with the sign always shown. */
export function signedPp(i18n: I18n, share: number): string {
  const n = share * 100;
  const s = i18n.dec(Math.abs(n), 1);
  return i18n.t("unit.pp", { n: n > 0 ? `+${s}` : n < 0 ? `−${s}` : s });
}

const deltaTone = (d: number | null): Tone | undefined =>
  d === null ? undefined : d >= NOTABLE ? "win" : d <= -NOTABLE ? "loss" : "muted";

/**
 * Allies or opponents. `total` is the owner's matches with metadata,
 * `friendMin` marks friends for the filter (allies only).
 */
export function peerTable(
  { i18n, look }: Ctx,
  opts: { label: string; peers: Peer[]; total: Bucket; ownerId: string; friendMin?: number; profiles?: Profiles },
): TableSpec {
  const { t } = i18n;
  const { peers, total, ownerId, friendMin } = opts;
  const profiles = opts.profiles ?? NO_PROFILES;
  return {
    label: opts.label,
    columns: [
      { key: "player", label: t("col.player") },
      { key: "matches", label: t("col.together"), numeric: true },
      { key: "winRate", label: t("col.winRate"), numeric: true },
      { key: "delta", label: t("col.wrDelta"), numeric: true },
      { key: "hero", label: t("col.topHero") },
      { key: "last", label: t("col.lastPlayed"), numeric: true },
    ],
    rows: peers.map((p) => {
      const { delta } = withWithout(p, total);
      const wr = winRate(p);
      const name = playerName(i18n, p.accountId, profiles);
      return {
        id: p.accountId,
        search: `${name} ${p.accountId}`,
        filters: friendMin ? { who: p.matches >= friendMin ? "friends" : "others" } : undefined,
        cells: {
          player: {
            text: name,
            href: `/players/${p.accountId}`,
            avatar: { src: avatarOf(p.accountId, profiles, "small"), name },
            tag: p.steamFriend ? t("friends.steamTag") : undefined,
            tagTitle: p.steamFriend ? t("friends.steamTagTitle") : undefined,
          },
          matches: { text: i18n.num(p.matches), sort: p.matches },
          winRate: { text: i18n.pct(p.wins, p.matches), sort: wr, bar: wr, tone: wr >= 0.5 ? "win" : undefined },
          delta: {
            text: delta === null ? "—" : signedPp(i18n, delta),
            sort: delta ?? 0,
            // A few meetings swing the rate by tens of points: no colour for noise.
            tone: p.matches >= MIN_SAMPLE ? deltaTone(delta) : "muted",
          },
          hero: { text: look.heroName(p.topHeroId), icon: look.heroIcon(p.topHeroId), sort: look.heroName(p.topHeroId) },
          last: {
            text: i18n.date(p.lastPlayed),
            sort: Number(p.lastPlayed),
            href: `/matches/${p.lastMatchId}/players/${ownerId}`,
            tone: "muted",
          },
        },
      };
    }),
    initialSort: { key: "matches", dir: "desc" },
    searchPlaceholder: t("filter.searchPlayer"),
    filters: friendMin
      ? [
          {
            key: "who",
            label: t("filter.who"),
            anyLabel: t("filter.all"),
            options: [
              { value: "friends", label: t("filter.friends") },
              { value: "others", label: t("filter.others") },
            ],
          },
        ]
      : undefined,
    collapseAfter: 10,
  };
}

/** Every hero seen with or against the owner, with the owner's win rates. */
export function matchupTable({ i18n, look }: Ctx, heroes: HeroMatchup[], total: Bucket): TableSpec {
  const { t } = i18n;
  const overall = winRate(total);
  const roles = [...new Set(heroes.map((h) => look.hero(h.heroId)?.type ?? "").filter(Boolean))].sort();
  const wrCell = (matches: number, wins: number) => {
    const b = { matches, wins };
    const d = matches ? winRate(b) - overall : 0;
    return {
      text: i18n.pct(wins, matches),
      sort: matches ? winRate(b) : -1,
      bar: matches ? winRate(b) : undefined,
      tone: matches >= MIN_SAMPLE ? deltaTone(d) : ("muted" as Tone),
    };
  };
  return {
    label: t("matchups.tableLabel"),
    columns: [
      { key: "hero", label: t("col.hero") },
      { key: "with", label: t("col.withMatches"), numeric: true },
      { key: "withWr", label: t("col.withWr"), numeric: true },
      { key: "against", label: t("col.againstMatches"), numeric: true },
      { key: "againstWr", label: t("col.againstWr"), numeric: true },
    ],
    rows: heroes.map((h) => {
      const name = look.heroName(h.heroId);
      return {
        id: String(h.heroId),
        search: name,
        filters: { role: look.hero(h.heroId)?.type ?? "" },
        cells: {
          hero: { text: name, icon: look.heroIcon(h.heroId) },
          with: { text: i18n.num(h.withMatches), sort: h.withMatches },
          withWr: wrCell(h.withMatches, h.withWins),
          against: { text: i18n.num(h.againstMatches), sort: h.againstMatches },
          againstWr: wrCell(h.againstMatches, h.againstWins),
        },
      };
    }),
    initialSort: { key: "against", dir: "desc" },
    searchPlaceholder: t("filter.searchHero"),
    filters: [
      {
        key: "role",
        label: t("filter.role"),
        anyLabel: t("filter.all"),
        options: roles.map((r) => ({ value: r, label: i18n.role(r) })),
      },
    ],
    collapseAfter: 10,
  };
}
