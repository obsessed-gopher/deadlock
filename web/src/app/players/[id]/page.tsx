import type { Metadata } from "next";
import Link from "next/link";
import { notFound, redirect } from "next/navigation";

import { api, isId, isNotFound } from "@/api/client.ts";
import type { MetaStatus, Player, SyncState } from "@/api/schema.gen.ts";
import { SyncButton } from "@/components/SyncButton.tsx";
import { dateTime, duration, gameModeName, kda, matchModeName, num, pct } from "@/lib/format.ts";
import { type Lookup, makeLookup } from "@/lib/gamedata.ts";
import { type HeroRow, heroRows } from "@/lib/heroes.ts";

import { syncPlayer } from "./actions.ts";

const PAGE_SIZE = 50;

type Params = { id: string };
type Search = { hero?: string; before?: string };

export async function generateMetadata({ params }: { params: Promise<Params> }): Promise<Metadata> {
  return { title: `Игрок ${(await params).id}` };
}

const syncStateText: Record<SyncState, string> = {
  SYNC_STATE_UNSPECIFIED: "—",
  SYNC_STATE_NEVER: "ещё не было",
  SYNC_STATE_RUNNING: "идёт",
  SYNC_STATE_IDLE: "готово",
  SYNC_STATE_ERROR: "ошибка",
};

const metaText: Record<MetaStatus, string> = {
  META_STATUS_UNSPECIFIED: "—",
  META_STATUS_PENDING: "в очереди",
  META_STATUS_OK: "",
  META_STATUS_EXPIRED: "реплей удалён",
  META_STATUS_FAILED: "не загрузилось",
};

export default async function PlayerPage({
  params,
  searchParams,
}: {
  params: Promise<Params>;
  searchParams: Promise<Search>;
}) {
  const { id } = await params;
  const { hero, before } = await searchParams;
  if (!isId(id)) notFound();

  let player: Player | null = null;
  try {
    player = await api.player(id);
  } catch (err) {
    if (!isNotFound(err)) throw err;
  }
  const action = syncPlayer.bind(null, id);

  if (!player) {
    return (
      <div className="card" style={{ maxWidth: 560 }}>
        <h1>Игрок {id}</h1>
        <p className="muted">Данных об этом игроке ещё нет. Синхронизация загрузит историю матчей.</p>
        <SyncButton action={action} />
      </div>
    );
  }
  // A SteamID64 in the URL resolves to the account id: keep one canonical URL.
  const canonical = String(player.accountId);
  if (canonical !== id) redirect(`/players/${canonical}`);

  const heroId = hero && /^\d+$/.test(hero) ? Number(hero) : 0;
  const [gd, page, heroStats] = await Promise.all([
    api.gameData(),
    api.matches(id, { pageSize: PAGE_SIZE, pageToken: before, heroId }),
    api.heroStats(id),
  ]);
  const look = makeLookup(gd);
  const losses = player.matchesTotal - player.wins;

  const nextHref = (() => {
    if (!page.nextPageToken) return "";
    const q = new URLSearchParams({ before: page.nextPageToken });
    if (heroId) q.set("hero", String(heroId));
    return `/players/${id}?${q}`;
  })();

  return (
    <>
      <div className="row spread">
        <div>
          <h1>Игрок {player.accountId}</h1>
          <div className="muted small">
            Синхронизация: {syncStateText[player.syncState]}, {dateTime(player.lastSyncedAt)}
            {player.syncError && <span className="loss"> · {player.syncError}</span>}
          </div>
        </div>
        <SyncButton action={action} />
      </div>

      <div className="stats">
        <Stat label="Матчей" value={num(player.matchesTotal)} />
        <Stat label="Победы / поражения" value={`${num(player.wins)} / ${num(losses)}`} />
        <Stat label="Винрейт" value={pct(player.wins, player.matchesTotal)} />
        <Stat
          label="С метаданными"
          value={num(player.matchesWithMeta)}
          hint={player.matchesPending > 0 ? `ещё ${num(player.matchesPending)} в очереди` : undefined}
        />
      </div>

      <h2>Герои</h2>
      <HeroTable rows={heroRows(heroStats.heroes)} look={look} accountId={id} selected={heroId} />

      <div className="row spread" style={{ marginTop: 28 }}>
        <h2 style={{ margin: 0 }}>Матчи</h2>
        <form method="get" className="row">
          <select name="hero" defaultValue={heroId ? String(heroId) : ""} aria-label="Герой">
            <option value="">Все герои</option>
            {look.playableHeroes.map((h) => (
              <option key={h.id} value={h.id}>
                {look.heroName(h.id)}
              </option>
            ))}
          </select>
          <button type="submit">Показать</button>
        </form>
      </div>

      <div className="table-wrap" style={{ marginTop: 10 }}>
        <table aria-label="Матчи">
          <thead>
            <tr>
              <th>Дата</th>
              <th>Герой</th>
              <th>Итог</th>
              <th className="num">K / D / A</th>
              <th className="num">Души</th>
              <th className="num">Добито</th>
              <th className="num">Ур.</th>
              <th className="num">Длит.</th>
              <th>Режим</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {page.matches.map((m) => {
              const ok = m.metaStatus === "META_STATUS_OK";
              return (
                <tr key={m.matchId}>
                  <td>{dateTime(m.startTime)}</td>
                  <td>{look.heroName(m.heroId)}</td>
                  <td className={m.won ? "win" : "loss"}>{m.won ? "Победа" : "Поражение"}</td>
                  <td className="num">{kda(m.kills, m.deaths, m.assists)}</td>
                  <td className="num">{num(m.netWorth)}</td>
                  <td className="num">{m.lastHits}</td>
                  <td className="num">{m.heroLevel}</td>
                  <td className="num">{duration(m.durationS)}</td>
                  <td className="muted small">
                    {gameModeName(m.gameMode)} · {matchModeName(m.matchMode)}
                  </td>
                  <td>
                    {ok ? (
                      <Link href={`/matches/${m.matchId}/players/${player.accountId}`}>разбор →</Link>
                    ) : (
                      <span className="muted small">{metaText[m.metaStatus]}</span>
                    )}
                  </td>
                </tr>
              );
            })}
            {page.matches.length === 0 && (
              <tr>
                <td colSpan={10} className="muted">
                  Матчей нет.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <div className="row pager">
        {before && (
          <Link className="button" href={heroId ? `/players/${id}?hero=${heroId}` : `/players/${id}`}>
            ← К последним
          </Link>
        )}
        {nextHref && (
          <Link className="button" href={nextHref}>
            Старше →
          </Link>
        )}
      </div>
    </>
  );
}

function HeroTable({
  rows,
  look,
  accountId,
  selected,
}: {
  rows: HeroRow[];
  look: Lookup;
  accountId: string;
  selected: number;
}) {
  if (rows.length === 0) return <p className="muted">Нет сыгранных матчей.</p>;
  return (
    <div className="table-wrap">
      <table aria-label="Герои">
        <thead>
          <tr>
            <th>Герой</th>
            <th className="num">Матчей</th>
            <th className="num">Винрейт</th>
            <th className="num">K / D / A в среднем</th>
            <th className="num">KDA</th>
            <th className="num">Души в минуту</th>
            <th>Последний матч</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.heroId} className={r.heroId === selected ? "me" : undefined}>
              <td>
                <Link href={`/players/${accountId}?hero=${r.heroId}`}>{look.heroName(r.heroId)}</Link>
              </td>
              <td className="num">{r.matches}</td>
              <td className={`num ${r.winRate >= 0.5 ? "win" : "loss"}`}>{pct(r.wins, r.matches)}</td>
              <td className="num">
                {r.avgKills.toFixed(1)} / {r.avgDeaths.toFixed(1)} / {r.avgAssists.toFixed(1)}
              </td>
              <td className="num">{r.kda.toFixed(2)}</td>
              <td className="num">{num(Math.round(r.soulsPerMin))}</td>
              <td className="muted">{dateTime(r.lastPlayed)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function Stat({ label, value, hint }: { label: string; value: string; hint?: string | undefined }) {
  return (
    <div className="card stat">
      <div className="label">{label}</div>
      <div className="value">{value}</div>
      {hint && <div className="muted small">{hint}</div>}
    </div>
  );
}
