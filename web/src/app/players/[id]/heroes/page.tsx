import type { Metadata } from "next";

import { api } from "@/api/client.ts";
import { DataTable } from "@/components/DataTable.tsx";
import { ErrorState } from "@/components/ErrorState.tsx";
import { tabHref } from "@/components/PlayerHead.tsx";
import { getI18n } from "@/i18n/server.ts";
import { attempt } from "@/lib/errors.ts";
import { makeLookup } from "@/lib/gamedata.ts";
import { heroTable } from "@/lib/views.ts";

import { loadProfile } from "../profile.tsx";

type Params = { id: string };

export async function generateMetadata({ params }: { params: Promise<Params> }): Promise<Metadata> {
  const { t } = await getI18n();
  return { title: `${t("tab.heroes")} · ${t("player.title", { id: (await params).id })}` };
}

export default async function HeroesPage({ params }: { params: Promise<Params> }) {
  const [{ id }, i18n] = await Promise.all([params, getI18n()]);
  const { t } = i18n;
  const profile = await loadProfile(i18n, id, "heroes");
  if (!profile.ok) return profile.page;
  const self = tabHref(id, "heroes");

  const [gd, heroes] = await Promise.all([attempt(() => api.gameData()), attempt(() => api.heroStats(id))]);
  if (!gd.ok) return <ErrorState i18n={i18n} kind={gd.kind} retryHref={self} />;
  if (!heroes.ok) return <ErrorState i18n={i18n} kind={heroes.kind} retryHref={self} />;
  const look = makeLookup(gd.data, i18n);

  return (
    <>
      {profile.head}
      <section aria-labelledby="heroes-title">
        <h2 id="heroes-title">
          {t("heroes.title")}
          <span className="count">{heroes.data.heroes.length}</span>
        </h2>
        {heroes.data.heroes.length === 0 ? (
          <p className="muted">{t("heroes.empty")}</p>
        ) : (
          <DataTable spec={heroTable({ i18n, look }, heroes.data.heroes, String(profile.player.accountId), 0)} />
        )}
      </section>
    </>
  );
}
