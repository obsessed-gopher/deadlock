import Link from "next/link";

import type { Match } from "@/api/schema.gen.ts";
import { kda, num, teamName } from "@/lib/format.ts";
import type { Lookup } from "@/lib/gamedata.ts";

/** Both teams of a match, sorted by net worth; the highlighted slot is marked. */
export function Scoreboard({ match, look, highlightSlot }: { match: Match; look: Lookup; highlightSlot?: number }) {
  const teams = [...new Set(match.players.map((p) => p.team))].sort((a, b) => a - b);
  return (
    <div className="grid-2">
      {teams.map((team) => {
        const players = match.players.filter((p) => p.team === team).sort((a, b) => b.netWorth - a.netWorth);
        const won = team === match.winningTeam;
        return (
          <div key={team} className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>
                    <span className={`team-${team}`}>{teamName(team)}</span>{" "}
                    <span className={won ? "win" : "loss"}>{won ? "победа" : "поражение"}</span>
                  </th>
                  <th className="num">K / D / A</th>
                  <th className="num">Души</th>
                  <th className="num">Урон</th>
                  <th className="num">Лечение</th>
                </tr>
              </thead>
              <tbody>
                {players.map((p) => (
                  <tr key={p.slot} className={p.slot === highlightSlot ? "me" : undefined}>
                    <td>
                      <Link href={`/matches/${match.matchId}/players/${p.accountId}`}>{look.heroName(p.heroId)}</Link>
                      {p.abandonTimeS > 0 && <span className="badge" title="Покинул матч"> ушёл</span>}
                    </td>
                    <td className="num">{kda(p.kills, p.deaths, p.assists)}</td>
                    <td className="num">{num(p.netWorth)}</td>
                    <td className="num">{num(p.playerDamage)}</td>
                    <td className="num">{num(p.playerHealing)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        );
      })}
    </div>
  );
}
