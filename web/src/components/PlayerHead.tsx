// Profile header shared by the profile tabs: name, sync state, summary line,
// sync button and the tab bar.
import type { Player, Profile } from "@/api/schema.gen.ts";
import type { SyncState } from "@/app/players/[id]/actions.ts";
import type { I18n } from "@/i18n/core.ts";

import { Avatar } from "./Avatar.tsx";
import { SyncButton } from "./SyncButton.tsx";

export const PROFILE_TABS = ["progress", "peers", "heroes", "matches"] as const;
export type ProfileTab = (typeof PROFILE_TABS)[number];

export function tabHref(accountId: string | number, tab: ProfileTab): string {
  return tab === "progress" ? `/players/${accountId}` : `/players/${accountId}/${tab}`;
}

export function PlayerHead({
  i18n,
  player: p,
  profile,
  tab,
  action,
  compareHref,
}: {
  i18n: I18n;
  player: Player;
  profile?: Profile;
  tab: ProfileTab;
  action: (prev: SyncState | null) => Promise<SyncState>;
  /** "Compare with me" target; omitted on the owner's own profile. */
  compareHref?: string;
}) {
  const { t } = i18n;
  const id = String(p.accountId);
  const name = profile?.personaName.trim() || t("player.title", { id });
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
        <div className="row player-id">
          <Avatar src={profile?.avatarFull} name={name} size={72} />
          <div className="stack">
          <span className="eyebrow">
            {t("player.account", { id })}
            {profile?.profileUrl && (
              <>
                {" · "}
                <a href={profile.profileUrl} target="_blank" rel="noopener noreferrer">
                  {t("player.steamProfile")}
                </a>
              </>
            )}
          </span>
          <h1>{name}</h1>
          <span className="muted">
            {t("player.summary", { matches: i18n.num(p.matchesTotal), winRate: i18n.pct(p.wins, p.matchesTotal) })}
          </span>
          <span className={p.syncState === "SYNC_STATE_ERROR" ? "tone-loss small" : "faint small"}>
            {syncLine}
            {p.matchesPending > 0 && ` · ${t("player.pending", { n: i18n.num(p.matchesPending) })}`}
          </span>
          </div>
        </div>
        <div className="row head-actions">
          {compareHref && (
            <a href={compareHref} className="btn">
              {t("compare.withMe")}
            </a>
          )}
          <SyncButton action={action} />
        </div>
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
