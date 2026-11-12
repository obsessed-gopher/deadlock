import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";

import { api, isId, isNotFound } from "@/api/client.ts";
import type { PlayerMatch, StatSnapshot } from "@/api/schema.gen.ts";
import { NetWorthChart } from "@/components/NetWorthChart.tsx";
import { Scoreboard } from "@/components/Scoreboard.tsx";
import { type BuildEntry, buildEntries, finalItems } from "@/lib/build.ts";
import { dateTime, duration, gameModeName, kda, kdaRatio, matchModeName, num, pct } from "@/lib/format.ts";
import { makeLookup } from "@/lib/gamedata.ts";

type Params = { matchId: string; accountId: string };

export async function generateMetadata({ params }: { params: Promise<Params> }): Promise<Metadata> {
  const { matchId, accountId } = await params;
  return { title: `Матч ${matchId} · ${accountId}` };
}

export default async function PlayerMatchPage({ params }: { params: Promise<Params> }) {
  const { matchId, accountId } = await params;
  if (!isId(matchId) || !isId(accountId)) notFound();
  let pm: PlayerMatch;
  try {
    pm = await api.playerMatch(matchId, accountId);
  } catch (err) {
    if (isNotFound(err)) notFound();
    throw err;
  }
  const match = pm.match;
  const me = match?.players.find((p) => p.slot === pm.slot);
  if (!match || !me) notFound();

  const look = makeLookup(await api.gameData());
  const won = me.team === match.winningTeam;
  const last: StatSnapshot | undefined = pm.timeline.at(-1);
  const entries = buildEntries(pm.items, look.ability);
  const teamOf = (slot: number) => match.players.find((p) => p.slot === slot)?.team ?? 0;
  const heroBySlot = (slot: number) => {
    const p = match.players.find((x) => x.slot === slot);
    return p ? look.heroName(p.heroId) : "—";
  };

  return (
    <>
      <div className="muted small">
        <Link href={`/players/${me.accountId}`}>Игрок {me.accountId}</Link> ·{" "}
        <Link href={`/matches/${match.matchId}`}>матч {match.matchId}</Link>
      </div>
      <h1>
        {look.heroName(me.heroId)} <span className={won ? "win" : "loss"}>{won ? "Победа" : "Поражение"}</span>
      </h1>
      <p className="muted">
        {dateTime(match.startTime)} · {duration(match.durationS)} · {gameModeName(match.gameMode)} ·{" "}
        {matchModeName(match.matchMode)}
      </p>

      <div className="stats">
        <Stat label="K / D / A" value={kda(me.kills, me.deaths, me.assists)} hint={`KDA ${kdaRatio(me.kills, me.deaths, me.assists)}`} />
        <Stat label="Души" value={num(me.netWorth)} hint={`${num(Math.round(me.netWorth / Math.max(1, match.durationS / 60)))} в минуту`} />
        <Stat label="Урон по героям" value={num(me.playerDamage)} />
        <Stat label="Лечение" value={num(me.playerHealing)} />
        <Stat label="Добито / отказано" value={`${me.lastHits} / ${me.denies}`} />
        {last && <Stat label="Точность" value={pct(last.shotsHit, last.shotsHit + last.shotsMissed)} />}
      </div>

      <h2>Души по времени</h2>
      <div className="card">
        <NetWorthChart series={pm.netWorthBySlot} teamOf={teamOf} highlightSlot={pm.slot} />
      </div>

      <h2>Таблица матча</h2>
      <Scoreboard match={match} look={look} highlightSlot={pm.slot} />

      <div className="grid-2">
        <div>
          <h2>Итоговые предметы</h2>
          <div className="items">
            {finalItems(entries).map((e) => (
              <span key={`${e.itemId}-${e.t}`} className={`item slot-${e.slot}`} title={`T${e.tier}, куплен на ${duration(e.t)}`}>
                {e.name}
              </span>
            ))}
          </div>
          {last && (
            <>
              <h2>Источники душ</h2>
              <SoulSources s={last} />
            </>
          )}
        </div>
        <div>
          <h2>Смерти ({pm.deaths.length})</h2>
          {pm.deaths.length === 0 ? (
            <p className="muted">Ни одной.</p>
          ) : (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Время</th>
                    <th>Убийца</th>
                    <th className="num">Время до смерти</th>
                    <th className="num">Возрождение</th>
                  </tr>
                </thead>
                <tbody>
                  {pm.deaths.map((d) => (
                    <tr key={d.t}>
                      <td>{duration(d.t)}</td>
                      <td>{heroBySlot(d.killerSlot)}</td>
                      <td className="num">{d.timeToKillS >= 0 ? `${d.timeToKillS.toFixed(1)} с` : "—"}</td>
                      <td className="num">{d.respawnS} с</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>

      <h2>Порядок покупок и улучшений</h2>
      <BuildTable entries={entries} />
    </>
  );
}

function Stat({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="card stat">
      <div className="label">{label}</div>
      <div className="value">{value}</div>
      {hint && <div className="muted small">{hint}</div>}
    </div>
  );
}

function SoulSources({ s }: { s: StatSnapshot }) {
  const rows: [string, number][] = [
    ["Крипы на линии", s.goldLaneCreep],
    ["Нейтралы", s.goldNeutralCreep],
    ["Герои", s.goldPlayer],
    ["Боссы", s.goldBoss],
    ["Сокровища", s.goldTreasure],
  ];
  const total = rows.reduce((sum, [, v]) => sum + v, 0);
  return (
    <div className="table-wrap">
      <table>
        <tbody>
          {rows.map(([name, v]) => (
            <tr key={name}>
              <td>{name}</td>
              <td className="num">{num(v)}</td>
              <td className="num muted">{pct(v, total)}</td>
            </tr>
          ))}
          <tr>
            <td className="loss">Потеряно при смертях</td>
            <td className="num loss">−{num(s.goldDeathLoss)}</td>
            <td />
          </tr>
        </tbody>
      </table>
    </div>
  );
}

const kindText = { item: "", unlock: "открыта", upgrade: "улучшение" } as const;

function BuildTable({ entries }: { entries: BuildEntry[] }) {
  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>Время</th>
            <th>Что</th>
            <th>Тип</th>
            <th className="num">Продан</th>
          </tr>
        </thead>
        <tbody>
          {entries.map((e, i) => (
            <tr key={i}>
              <td>{duration(e.t)}</td>
              <td className={e.kind === "item" ? `slot-${e.slot}` : undefined}>
                <span className={e.soldT ? "sold" : undefined}>{e.name}</span>
              </td>
              <td className="muted small">
                {e.kind === "item" ? (e.tier ? `предмет T${e.tier}` : "предмет") : kindText[e.kind]}
                {e.kind === "upgrade" && ` ${e.level}`}
              </td>
              <td className="num muted">{e.soldT ? duration(e.soldT) : ""}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
