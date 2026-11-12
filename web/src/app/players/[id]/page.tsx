import type { Metadata } from "next";
import { notFound, redirect } from "next/navigation";

import { api, isId } from "@/api/client.ts";
import { DataTable } from "@/components/DataTable.tsx";
import { ErrorState } from "@/components/ErrorState.tsx";
import { MatchesTable } from "@/components/MatchesTable.tsx";
import { Stat } from "@/components/Stat.tsx";
import { SyncButton } from "@/components/SyncButton.tsx";
import { getI18n } from "@/i18n/server.ts";
import { attempt } from "@/lib/errors.ts";
import { makeLookup } from "@/lib/gamedata.ts";
import { matchStateQuery, parseMatchState } from "@/lib/matchQuery.ts";
import { heroTable } from "@/lib/views.ts";

import { syncPlayer } from "./actions.ts";

type Params = { id: string };
type Search = Record<string, string | string[] | undefined>;

export async function generateMetadata({ params }: { params: Promise<Params> }): Promise<Metadata> {
  const { t } = await getI18n();
  return { title: t("player.title", { id: (await params).id }) };
}

export default async function PlayerPage({ params, searchParams }: { params: Promise<Params>; searchParams: Promise<Search> }) {
  const [{ id }, sp, i18n] = await Promise.all([params, searchParams, getI18n()]);
  const { t } = i18n;
  if (!isId(id)) notFound();
  const state = parseMatchState(sp);
  const self = `/players/${id}${matchStateQuery(state)}`;

  const player = await attempt(() => api.player(id));
  const action = syncPlayer.bind(null, id);
  if (!player.ok && player.kind === "notFound") {
    return (
      <section className="state">
        <span className="eyebrow">{t("player.account", { id })}</span>
        <h1>{t("player.title", { id })}</h1>
        <p>{t("player.noData")}</p>
        <SyncButton action={action} />
      </section>
    );
  }
  if (!player.ok) return <ErrorState i18n={i18n} kind={player.kind} retryHref={self} />;
  const p = player.data;

  // A SteamID64 in the URL resolves to the account id: keep one canonical URL.
  const canonical = String(p.accountId);
  if (canonical !== id) redirect(`/players/${canonical}${matchStateQuery(state)}`);

  const [gd, page, heroes] = await Promise.all([
    attempt(() => api.gameData()),
    attempt(() => api.matches(id, state)),
    attempt(() => api.heroStats(id)),
  ]);
  if (!gd.ok) return <ErrorState i18n={i18n} kind={gd.kind} retryHref={self} />;
  if (!page.ok) return <ErrorState i18n={i18n} kind={page.kind} retryHref={self} />;
  if (!heroes.ok) return <ErrorState i18n={i18n} kind={heroes.kind} retryHref={self} />;

  const look = makeLookup(gd.data, i18n);
  const syncLine =
    p.syncState === "SYNC_STATE_RUNNING"
      ? t("player.syncRunning")
      : p.syncState === "SYNC_STATE_ERROR"
        ? t("player.syncError", { reason: p.syncError })
        : Number(p.lastSyncedAt) > 0
          ? t("player.synced", { date: i18n.date(p.lastSyncedAt) })
          : t("player.neverSynced");

  return (
    <>
      <section>
        <div className="head-row">
          <div className="stack">
            <span className="eyebrow">{t("player.account", { id: canonical })}</span>
            <h1>{t("player.title", { id: canonical })}</h1>
            <span className={p.syncState === "SYNC_STATE_ERROR" ? "tone-loss" : "muted"}>
              {syncLine}
              {p.matchesPending > 0 && ` · ${t("player.pending", { n: i18n.num(p.matchesPending) })}`}
            </span>
          </div>
          <SyncButton action={action} />
        </div>
        <div className="stats">
          <Stat label={t("stat.matches")} value={i18n.num(p.matchesTotal)} />
          <Stat label={t("stat.winsLosses")} value={`${i18n.num(p.wins)} / ${i18n.num(p.matchesTotal - p.wins)}`} />
          <Stat
            label={t("stat.winRate")}
            value={i18n.pct(p.wins, p.matchesTotal)}
            meter={p.matchesTotal ? p.wins / p.matchesTotal : 0}
          />
          <Stat label={t("stat.withMeta")} value={i18n.num(p.matchesWithMeta)} />
        </div>
      </section>

      <section aria-labelledby="heroes-title">
        <h2 id="heroes-title">
          {t("heroes.title")}
          <span className="count">{heroes.data.heroes.length}</span>
        </h2>
        {heroes.data.heroes.length === 0 ? (
          <p className="muted">{t("heroes.empty")}</p>
        ) : (
          <DataTable spec={heroTable({ i18n, look }, heroes.data.heroes, canonical, state.hero)} />
        )}
      </section>

      <MatchesTable
        i18n={i18n}
        look={look}
        base={`/players/${canonical}`}
        accountId={p.accountId}
        state={state}
        page={page.data}
      />
    </>
  );
}
