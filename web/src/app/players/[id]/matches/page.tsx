import type { Metadata } from "next";

import { api } from "@/api/client.ts";
import { ErrorState } from "@/components/ErrorState.tsx";
import { MatchesTable } from "@/components/MatchesTable.tsx";
import { tabHref } from "@/components/PlayerHead.tsx";
import { getI18n } from "@/i18n/server.ts";
import { attempt } from "@/lib/errors.ts";
import { makeLookup } from "@/lib/gamedata.ts";
import { matchStateQuery, parseMatchState } from "@/lib/matchQuery.ts";

import { loadProfile } from "../profile.tsx";

type Params = { id: string };
type Search = Record<string, string | string[] | undefined>;

export async function generateMetadata({ params }: { params: Promise<Params> }): Promise<Metadata> {
  const { t } = await getI18n();
  return { title: `${t("tab.matches")} · ${t("player.title", { id: (await params).id })}` };
}

export default async function MatchesPage({ params, searchParams }: { params: Promise<Params>; searchParams: Promise<Search> }) {
  const [{ id }, sp, i18n] = await Promise.all([params, searchParams, getI18n()]);
  const state = parseMatchState(sp);
  const query = matchStateQuery(state);
  const profile = await loadProfile(i18n, id, "matches", query);
  if (!profile.ok) return profile.page;
  const self = tabHref(id, "matches") + query;

  const [gd, page] = await Promise.all([attempt(() => api.gameData()), attempt(() => api.matches(id, state))]);
  if (!gd.ok) return <ErrorState i18n={i18n} kind={gd.kind} retryHref={self} />;
  if (!page.ok) return <ErrorState i18n={i18n} kind={page.kind} retryHref={self} />;

  return (
    <>
      {profile.head}
      <MatchesTable
        i18n={i18n}
        look={makeLookup(gd.data, i18n)}
        base={tabHref(profile.player.accountId, "matches")}
        accountId={profile.player.accountId}
        state={state}
        page={page.data}
      />
    </>
  );
}
