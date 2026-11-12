import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { api, isId, isNotFound } from "@/api/client.ts";
import type { Match } from "@/api/schema.gen.ts";
import { Scoreboard } from "@/components/Scoreboard.tsx";
import { dateTime, duration, gameModeName, matchModeName } from "@/lib/format.ts";
import { makeLookup } from "@/lib/gamedata.ts";

type Params = { matchId: string };

export async function generateMetadata({ params }: { params: Promise<Params> }): Promise<Metadata> {
  return { title: `Матч ${(await params).matchId}` };
}

export default async function MatchPage({ params }: { params: Promise<Params> }) {
  const { matchId } = await params;
  if (!isId(matchId)) notFound();
  let match: Match;
  try {
    match = await api.match(matchId);
  } catch (err) {
    if (isNotFound(err)) notFound();
    throw err;
  }
  const look = makeLookup(await api.gameData());
  return (
    <>
      <h1>Матч {match.matchId}</h1>
      <p className="muted">
        {dateTime(match.startTime)} · {duration(match.durationS)} · {gameModeName(match.gameMode)} ·{" "}
        {matchModeName(match.matchMode)}
      </p>
      {match.players.length > 0 ? (
        <Scoreboard match={match} look={look} />
      ) : (
        <p className="muted">Метаданные матча ещё не загружены.</p>
      )}
    </>
  );
}
