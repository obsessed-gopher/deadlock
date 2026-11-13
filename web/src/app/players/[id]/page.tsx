import type { Metadata } from "next";
import { redirect } from "next/navigation";

import { api } from "@/api/client.ts";
import { ErrorState } from "@/components/ErrorState.tsx";
import { tabHref } from "@/components/PlayerHead.tsx";
import { QueueNav } from "@/components/QueueNav.tsx";
import {
  FormCard,
  LobbyChart,
  RecordsSection,
  SessionsSection,
  SpeedSection,
  TREND_METRICS,
  type TrendMetric,
  TrendChart,
} from "@/components/Progress.tsx";
import { getI18n } from "@/i18n/server.ts";
import { attempt } from "@/lib/errors.ts";
import { makeLookup } from "@/lib/gamedata.ts";
import { matchStateQuery, parseMatchState } from "@/lib/matchQuery.ts";
import { type QueueKey, countQueues, parseQueue } from "@/lib/queue.ts";
import { inQueue } from "@/lib/seen.ts";
import {
  afterStreaks,
  aggregate,
  form,
  hours,
  minHeroMatches,
  months,
  records,
  sessions,
  speed,
} from "@/lib/progress.ts";

import { loadProfile } from "./profile.tsx";

type Params = { id: string };
type Search = Record<string, string | string[] | undefined>;

/** Months on the trend chart. */
const TREND_MONTHS = 18;

export async function generateMetadata({ params }: { params: Promise<Params> }): Promise<Metadata> {
  const { t } = await getI18n();
  return { title: t("player.title", { id: (await params).id }) };
}

export default async function ProgressPage({ params, searchParams }: { params: Promise<Params>; searchParams: Promise<Search> }) {
  const [{ id }, sp, i18n] = await Promise.all([params, searchParams, getI18n()]);
  const { t } = i18n;
  // The match list used to live here: keep its old links working.
  const listQuery = matchStateQuery(parseMatchState(sp));
  if (listQuery) redirect(tabHref(id, "matches") + listQuery);

  const metric: TrendMetric = TREND_METRICS.find((m) => m === sp.trend) ?? "spm";
  const queue = parseQueue(sp.queue);
  const queryOf = (m: TrendMetric, q: QueueKey | "") => {
    const p = new URLSearchParams();
    if (m !== "spm") p.set("trend", m);
    if (q) p.set("queue", q);
    const s = p.toString();
    return s ? `?${s}` : "";
  };
  const query = queryOf(metric, queue);
  const profile = await loadProfile(i18n, id, "progress", query);
  if (!profile.ok) return profile.page;
  const accountId = profile.player.accountId;
  const self = tabHref(accountId, "progress") + query;

  const [gd, facts] = await Promise.all([attempt(() => api.gameData()), attempt(() => api.facts(id))]);
  if (!gd.ok) return <ErrorState i18n={i18n} kind={gd.kind} retryHref={self} />;
  if (!facts.ok) return <ErrorState i18n={i18n} kind={facts.kind} retryHref={self} />;

  const look = makeLookup(gd.data, i18n);
  const all = facts.data.matches;
  const fs = inQueue(all, queue);
  const nav = (
    <QueueNav
      i18n={i18n}
      counts={countQueues(all)}
      current={queue}
      hrefFor={(q) => tabHref(accountId, "progress") + queryOf(metric, q)}
    />
  );
  if (fs.length === 0) {
    return (
      <>
        {profile.head}
        {nav}
        <p className="muted">{t("progress.empty")}</p>
      </>
    );
  }
  // Dates render on the server: its time zone is the user's (TZ in .env).
  const tz = process.env.TZ || Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  const ms = months(fs, tz);
  const streaks = afterStreaks(fs);

  return (
    <>
      {profile.head}
      {nav}
      <FormCard i18n={i18n} form={form(fs)} />
      <TrendChart
        i18n={i18n}
        months={ms.slice(-TREND_MONTHS)}
        metric={metric}
        hrefFor={(m) => tabHref(accountId, "progress") + queryOf(m, queue)}
      />
      <LobbyChart i18n={i18n} months={ms.slice(-TREND_MONTHS)} />
      <SpeedSection
        i18n={i18n}
        look={look}
        accountId={accountId}
        speed={speed(fs, facts.data.soulMarksS, facts.data.soulTargets, {
          minHeroMatches: minHeroMatches(fs.filter((f) => f.hasMeta).length),
        })}
      />
      <SessionsSection
        i18n={i18n}
        sessions={sessions(fs)}
        hours={hours(fs, tz)}
        afterLosses={streaks.afterLosses}
        overall={aggregate(fs).winRate}
        tz={tz}
      />
      <RecordsSection i18n={i18n} look={look} records={records(fs)} accountId={accountId} />
    </>
  );
}
