import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { api, isId } from "@/api/client.ts";
import { DataTable } from "@/components/DataTable.tsx";
import { ErrorState } from "@/components/ErrorState.tsx";
import { getI18n } from "@/i18n/server.ts";
import { attempt } from "@/lib/errors.ts";
import { makeLookup } from "@/lib/gamedata.ts";
import { scoreboardTables } from "@/lib/views.ts";

type Params = { matchId: string };

export async function generateMetadata({ params }: { params: Promise<Params> }): Promise<Metadata> {
  const { t } = await getI18n();
  return { title: t("match.title", { id: (await params).matchId }) };
}

export default async function MatchPage({ params }: { params: Promise<Params> }) {
  const [{ matchId }, i18n] = await Promise.all([params, getI18n()]);
  const { t } = i18n;
  if (!isId(matchId)) notFound();
  const [match, gd] = await Promise.all([attempt(() => api.match(matchId)), attempt(() => api.gameData())]);
  if (!match.ok && match.kind === "notFound") notFound();
  if (!match.ok) return <ErrorState i18n={i18n} kind={match.kind} retryHref={`/matches/${matchId}`} />;
  if (!gd.ok) return <ErrorState i18n={i18n} kind={gd.kind} retryHref={`/matches/${matchId}`} />;
  const m = match.data;
  const look = makeLookup(gd.data, i18n);
  const profiles = await api.profiles(m.players.map((p) => p.accountId));
  return (
    <>
      <section>
        <span className="eyebrow">{i18n.gameMode(m.gameMode)}</span>
        <h1>{t("match.title", { id: m.matchId })}</h1>
        <span className="muted">
          {i18n.date(m.startTime)} · {i18n.duration(m.durationS)} · {i18n.matchMode(m.matchMode)}
        </span>
      </section>
      <section aria-labelledby="scoreboard-title">
        <h2 id="scoreboard-title">{t("match.scoreboard")}</h2>
        {m.players.length > 0 ? (
          <div className="stack gap-16">
            {scoreboardTables({ i18n, look }, m, undefined, profiles).map((spec) => (
              <DataTable key={spec.label} spec={spec} />
            ))}
          </div>
        ) : (
          <p className="muted">{t("match.notLoaded")}</p>
        )}
      </section>
    </>
  );
}
