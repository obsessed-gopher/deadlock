import type { Metadata } from "next";
import { notFound, redirect } from "next/navigation";

import { api } from "@/api/client.ts";
import { CompareHead, HeroesSideBySide, MetricsTable, SharedCard, type Side } from "@/components/Compare.tsx";
import { DataTable } from "@/components/DataTable.tsx";
import { ErrorState } from "@/components/ErrorState.tsx";
import { QueueNav } from "@/components/QueueNav.tsx";
import { getI18n } from "@/i18n/server.ts";
import { compareHref, sharedTable } from "@/lib/compareViews.ts";
import { attempt } from "@/lib/errors.ts";
import { makeLookup } from "@/lib/gamedata.ts";
import { playerName } from "@/lib/profiles.ts";
import { type QueueKey, countQueues, parseQueue } from "@/lib/queue.ts";
import { heroLines, inQueue, shared, summarize } from "@/lib/seen.ts";
import { toAccountId } from "@/lib/tracked.ts";

type Params = { a: string; b: string };
type Search = Record<string, string | string[] | undefined>;

export async function generateMetadata(): Promise<Metadata> {
  const { t } = await getI18n();
  return { title: t("compare.title") };
}

export default async function ComparePage({ params, searchParams }: { params: Promise<Params>; searchParams: Promise<Search> }) {
  const [{ a: rawA, b: rawB }, sp, i18n] = await Promise.all([params, searchParams, getI18n()]);
  const { t } = i18n;
  const a = toAccountId(rawA);
  const b = toAccountId(rawB);
  if (!a || !b) notFound();
  const queue = parseQueue(sp.queue);
  // SteamID64 in the URL: same page under account ids.
  if (a !== rawA || b !== rawB) redirect(compareHref(a, b, queue));
  const self = compareHref(a, b, queue);

  const [gd, seenA, seenB, profiles] = await Promise.all([
    attempt(() => api.gameData()),
    attempt(() => api.seen(a)),
    attempt(() => api.seen(b)),
    api.profiles([a, b]),
  ]);
  if (!gd.ok) return <ErrorState i18n={i18n} kind={gd.kind} retryHref={self} />;
  if (!seenA.ok) return <ErrorState i18n={i18n} kind={seenA.kind} retryHref={self} />;
  if (!seenB.ok) return <ErrorState i18n={i18n} kind={seenB.kind} retryHref={self} />;

  const look = makeLookup(gd.data, i18n);
  const allA = seenA.data.matches;
  const allB = seenB.data.matches;
  const msA = inQueue(allA, queue);
  const msB = inQueue(allB, queue);
  const side = (id: string, ms: typeof msA): Side => ({
    id,
    name: playerName(i18n, id, profiles),
    profile: profiles.get(id),
    summary: summarize(ms),
    heroes: heroLines(ms),
  });
  const A = side(a, msA);
  const B = side(b, msB);
  const both = shared(msA, msB);
  const counts = countQueues(allA);
  const countsB = countQueues(allB);
  for (const q of Object.keys(counts) as QueueKey[]) counts[q] = Math.max(counts[q], countsB[q]);

  return (
    <>
      <CompareHead i18n={i18n} a={A} b={B} />
      {a === b && <p className="muted">{t("compare.same")}</p>}
      <QueueNav i18n={i18n} counts={counts} current={queue} hrefFor={(q) => compareHref(a, b, q)} />
      {A.summary.matches === 0 && B.summary.matches === 0 ? (
        <p className="muted">{t("compare.empty")}</p>
      ) : (
        <>
          <SharedCard i18n={i18n} shared={both} a={A} b={B} />
          <MetricsTable i18n={i18n} a={A} b={B} />
          <HeroesSideBySide i18n={i18n} look={look} a={A} b={B} />
          {both.matches.length > 0 && (
            <section aria-label={t("compare.shared")}>
              <DataTable spec={sharedTable({ i18n, look }, both.matches, { a: A.name, b: B.name, aId: a })} />
            </section>
          )}
        </>
      )}
    </>
  );
}
