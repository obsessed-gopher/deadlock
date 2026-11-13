// Loading shared by the profile tabs: invalid ids are 404, an unknown player
// gets the sync prompt, API failures a localized error page, and a SteamID64
// in the URL redirects to the canonical account id of the same tab.
import { notFound, redirect } from "next/navigation";
import type { ReactNode } from "react";

import { api, isId } from "@/api/client.ts";
import type { Player, Profile as SteamProfile } from "@/api/schema.gen.ts";
import { ErrorState } from "@/components/ErrorState.tsx";
import { PlayerHead, type ProfileTab, tabHref } from "@/components/PlayerHead.tsx";
import { SyncButton } from "@/components/SyncButton.tsx";
import type { I18n } from "@/i18n/core.ts";
import { attempt } from "@/lib/errors.ts";

import { syncPlayer } from "./actions.ts";

export type Profile =
  | { ok: true; player: Player; profile?: SteamProfile; head: ReactNode }
  | { ok: false; page: ReactNode };

/** query: the tab's own URL state ("?sort=..."), kept on redirects and retries. */
export async function loadProfile(i18n: I18n, id: string, tab: ProfileTab, query = ""): Promise<Profile> {
  const { t } = i18n;
  if (!isId(id)) notFound();
  const self = tabHref(id, tab) + query;
  const action = syncPlayer.bind(null, id);

  const [player, profiles] = await Promise.all([attempt(() => api.player(id)), api.profiles([id])]);
  const profile = [...profiles.values()][0];
  if (!player.ok && player.kind === "notFound") {
    return {
      ok: false,
      page: (
        <section className="state">
          <span className="eyebrow">{t("player.account", { id })}</span>
          <h1>{profile?.personaName.trim() || t("player.title", { id })}</h1>
          <p>{t("player.noData")}</p>
          <SyncButton action={action} />
        </section>
      ),
    };
  }
  if (!player.ok) return { ok: false, page: <ErrorState i18n={i18n} kind={player.kind} retryHref={self} /> };

  const canonical = String(player.data.accountId);
  if (canonical !== id) redirect(tabHref(canonical, tab) + query);
  return {
    ok: true,
    player: player.data,
    profile,
    head: <PlayerHead i18n={i18n} player={player.data} profile={profile} tab={tab} action={action} />,
  };
}
