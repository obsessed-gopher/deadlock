import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";

import { api, isId } from "@/api/client.ts";
import { DataTable } from "@/components/DataTable.tsx";
import { ErrorState } from "@/components/ErrorState.tsx";
import { Icon } from "@/components/Icon.tsx";
import { NetWorthChart } from "@/components/NetWorthChart.tsx";
import { SoulBars } from "@/components/SoulBars.tsx";
import { Stat } from "@/components/Stat.tsx";
import { getI18n } from "@/i18n/server.ts";
import { buildEntries, finalItems } from "@/lib/build.ts";
import { attempt } from "@/lib/errors.ts";
import { kda, kdaRatio } from "@/lib/format.ts";
import { makeLookup } from "@/lib/gamedata.ts";
import { tileOf } from "@/lib/icons.ts";
import { buildTable, deathsTable, scoreboardTables } from "@/lib/views.ts";

type Params = { matchId: string; accountId: string };

export async function generateMetadata({ params }: { params: Promise<Params> }): Promise<Metadata> {
  const { t } = await getI18n();
  const { matchId, accountId } = await params;
  return { title: `${t("match.title", { id: matchId })} · ${accountId}` };
}

export default async function PlayerMatchPage({ params }: { params: Promise<Params> }) {
  const [{ matchId, accountId }, i18n] = await Promise.all([params, getI18n()]);
  const { t } = i18n;
  if (!isId(matchId) || !isId(accountId)) notFound();
  const self = `/matches/${matchId}/players/${accountId}`;
  const [res, gd] = await Promise.all([attempt(() => api.playerMatch(matchId, accountId)), attempt(() => api.gameData())]);
  if (!res.ok && res.kind === "notFound") notFound();
  if (!res.ok) return <ErrorState i18n={i18n} kind={res.kind} retryHref={self} />;
  if (!gd.ok) return <ErrorState i18n={i18n} kind={gd.kind} retryHref={self} />;
  const pm = res.data;
  const match = pm.match;
  const me = match?.players.find((p) => p.slot === pm.slot);
  if (!match || !me) notFound();

  const look = makeLookup(gd.data, i18n);
  const ctx = { i18n, look };
  const won = me.team === match.winningTeam;
  const last = pm.timeline.at(-1);
  const entries = buildEntries(pm.items, look.ability, look.abilityName);
  const teamOf = (slot: number) => match.players.find((p) => p.slot === slot)?.team ?? 0;
  const perMin = Math.round(me.netWorth / Math.max(1, match.durationS / 60));

  return (
    <>
      <section>
        <nav className="crumbs" aria-label={t("app.title")}>
          <Link href={`/players/${me.accountId}`}>{t("player.title", { id: me.accountId })}</Link>
          <span aria-hidden="true">/</span>
          <Link href={`/matches/${match.matchId}`}>{t("match.title", { id: match.matchId })}</Link>
        </nav>
        <div className="row" style={{ gap: 16 }}>
          {look.heroIcon(me.heroId) && <Icon src={look.heroIcon(me.heroId)!} size={56} />}
          <h1>{look.heroName(me.heroId)}</h1>
          <span className={`badge ${won ? "tone-win" : "tone-loss"}`}>
            <span className="dot" aria-hidden="true" />
            {won ? t("result.win") : t("result.loss")}
          </span>
        </div>
        <span className="muted">
          {i18n.date(match.startTime)} · {i18n.duration(match.durationS)} · {i18n.gameMode(match.gameMode)} ·{" "}
          {i18n.matchMode(match.matchMode)} · <span className={`tone-team-${me.team}`}>{i18n.team(me.team)}</span>
        </span>
        <div className="stats">
          <Stat
            label={t("stat.kda")}
            value={kda(me.kills, me.deaths, me.assists)}
            hint={t("stat.kdaRatio", { n: i18n.dec(kdaRatio(me.kills, me.deaths, me.assists), 2) })}
          />
          <Stat label={t("stat.souls")} value={i18n.num(me.netWorth)} hint={t("stat.perMin", { n: i18n.num(perMin) })} />
          <Stat label={t("stat.damage")} value={i18n.num(me.playerDamage)} />
          <Stat label={t("stat.healing")} value={i18n.num(me.playerHealing)} />
          <Stat label={t("stat.lastHitsDenies")} value={`${me.lastHits} / ${me.denies}`} />
          {last && <Stat label={t("stat.accuracy")} value={i18n.pct(last.shotsHit, last.shotsHit + last.shotsMissed)} />}
        </div>
      </section>

      <section aria-labelledby="chart-title">
        <h2 id="chart-title">{t("match.chart")}</h2>
        <div className="card stack">
          <NetWorthChart i18n={i18n} series={pm.netWorthBySlot} teamOf={teamOf} highlightSlot={pm.slot} />
        </div>
      </section>

      <section aria-labelledby="scoreboard-title">
        <h2 id="scoreboard-title">{t("match.scoreboard")}</h2>
        <div className="grid-2">
          {scoreboardTables(ctx, match, pm.slot).map((spec) => (
            <DataTable key={spec.label} spec={spec} />
          ))}
        </div>
      </section>

      <div className="grid-2">
        <section aria-labelledby="items-title">
          <h2 id="items-title">{t("match.finalItems")}</h2>
          <div className="items">
            {finalItems(entries).map((e) => (
              <span key={`${e.itemId}-${e.t}`} className="item" title={i18n.duration(e.t)}>
                {look.abilityIcon(e.itemId) ? (
                  <Icon src={look.abilityIcon(e.itemId)!} tile={tileOf(e.slot, true)} size={28} />
                ) : (
                  <span className={`slot-mark slot-${e.slot || "ability"}`} aria-hidden="true" />
                )}
                {e.name}
                {e.tier > 0 && <span className="tier">T{e.tier}</span>}
              </span>
            ))}
          </div>
          {last && (
            <>
              <h2>{t("match.souls")}</h2>
              <SoulBars i18n={i18n} s={last} />
            </>
          )}
        </section>
        <section aria-labelledby="deaths-title">
          <h2 id="deaths-title">
            {t("match.deaths")}
            <span className="count">{pm.deaths.length}</span>
          </h2>
          {pm.deaths.length === 0 ? (
            <p className="muted">{t("match.noDeaths")}</p>
          ) : (
            <DataTable spec={deathsTable(ctx, pm.deaths, match.players)} />
          )}
        </section>
      </div>

      <section aria-labelledby="build-title">
        <h2 id="build-title">
          {t("match.build")}
          <span className="count">{entries.length}</span>
        </h2>
        <DataTable spec={buildTable(ctx, entries)} />
      </section>
    </>
  );
}
