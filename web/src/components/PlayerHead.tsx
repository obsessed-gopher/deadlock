// Profile header shared by the profile tabs: name, sync state, summary line,
// sync button and the tab bar.
import type { Player } from "@/api/schema.gen.ts";
import type { SyncState } from "@/app/players/[id]/actions.ts";
import type { I18n } from "@/i18n/core.ts";

import { SyncButton } from "./SyncButton.tsx";

export const PROFILE_TABS = ["progress", "peers", "heroes", "matches"] as const;
export type ProfileTab = (typeof PROFILE_TABS)[number];

export function tabHref(accountId: string | number, tab: ProfileTab): string {
  return tab === "progress" ? `/players/${accountId}` : `/players/${accountId}/${tab}`;
}

export function PlayerHead({
  i18n,
  player: p,
  tab,
  action,
}: {
  i18n: I18n;
  player: Player;
  tab: ProfileTab;
  action: (prev: SyncState | null) => Promise<SyncState>;
}) {
  const { t } = i18n;
  const id = String(p.accountId);
  const syncLine =
    p.syncState === "SYNC_STATE_RUNNING"
      ? t("player.syncRunning")
      : p.syncState === "SYNC_STATE_ERROR"
        ? t("player.syncError", { reason: p.syncError })
        : Number(p.lastSyncedAt) > 0
          ? t("player.synced", { date: i18n.date(p.lastSyncedAt) })
          : t("player.neverSynced");
  return (
    <section className="profile-head">
      <div className="head-row">
        <div className="stack">
          <span className="eyebrow">{t("player.account", { id })}</span>
          <h1>{t("player.title", { id })}</h1>
          <span className="muted">
            {t("player.summary", { matches: i18n.num(p.matchesTotal), winRate: i18n.pct(p.wins, p.matchesTotal) })}
          </span>
          <span className={p.syncState === "SYNC_STATE_ERROR" ? "tone-loss small" : "faint small"}>
            {syncLine}
            {p.matchesPending > 0 && ` · ${t("player.pending", { n: i18n.num(p.matchesPending) })}`}
          </span>
        </div>
        <SyncButton action={action} />
      </div>
      <nav className="tabs" aria-label={t("tab.label")}>
        {PROFILE_TABS.map((k) => (
          <a key={k} href={tabHref(id, k)} aria-current={k === tab ? "page" : undefined}>
            {t(`tab.${k}`)}
          </a>
        ))}
      </nav>
    </section>
  );
}
