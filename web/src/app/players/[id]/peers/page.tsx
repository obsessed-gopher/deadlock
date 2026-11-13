import type { Metadata } from "next";

import { api } from "@/api/client.ts";
import { DataTable } from "@/components/DataTable.tsx";
import { ErrorState } from "@/components/ErrorState.tsx";
import { FriendsSection, MatchupEdges, PartySection } from "@/components/Peers.tsx";
import { tabHref } from "@/components/PlayerHead.tsx";
import { getI18n } from "@/i18n/server.ts";
import { attempt } from "@/lib/errors.ts";
import { makeLookup } from "@/lib/gamedata.ts";
import { matchupTable, peerTable } from "@/lib/peerViews.ts";
import { friends, heroEdges, minMatchup, party } from "@/lib/peers.ts";

import { loadProfile } from "../profile.tsx";

type Params = { id: string };

export async function generateMetadata({ params }: { params: Promise<Params> }): Promise<Metadata> {
  const { t } = await getI18n();
  return { title: `${t("tab.peers")} · ${t("player.title", { id: (await params).id })}` };
}

export default async function PeersPage({ params }: { params: Promise<Params> }) {
  const [{ id }, i18n] = await Promise.all([params, getI18n()]);
  const { t } = i18n;
  const profile = await loadProfile(i18n, id, "peers");
  if (!profile.ok) return profile.page;
  const ownerId = String(profile.player.accountId);
  const self = tabHref(ownerId, "peers");

  const [gd, peers] = await Promise.all([attempt(() => api.gameData()), attempt(() => api.peers(id))]);
  if (!gd.ok) return <ErrorState i18n={i18n} kind={gd.kind} retryHref={self} />;
  if (!peers.ok) return <ErrorState i18n={i18n} kind={peers.kind} retryHref={self} />;

  const p = peers.data;
  if (p.matches === 0) {
    return (
      <>
        {profile.head}
        <p className="muted">{t("peers.empty")}</p>
      </>
    );
  }
  const look = makeLookup(gd.data, i18n);
  const profiles = await api.profiles([...p.allies, ...p.enemies].map((x) => x.accountId));
  const total = { matches: p.matches, wins: p.wins };
  const min = minMatchup(p.matches);
  const ctx = { i18n, look };

  return (
    <>
      {profile.head}
      <PartySection
        i18n={i18n}
        party={party(p.partySizes)}
        friendMin={p.friendMinMatches}
        sub={t("peers.sub", { n: i18n.num(p.matches) })}
      />
      <FriendsSection
        {...ctx}
        friends={friends(p.allies, p.friendMinMatches)}
        total={total}
        ownerId={ownerId}
        friendMin={p.friendMinMatches}
        profiles={profiles}
      />
      <section aria-labelledby="matchups-title">
        <h2 id="matchups-title">{t("matchups.title")}</h2>
        <MatchupEdges {...ctx} edges={heroEdges(p.heroes, total, min)} total={total} min={min} />
        <DataTable spec={matchupTable(ctx, p.heroes, total)} />
      </section>
      <section aria-labelledby="allies-title">
        <div className="section-head">
          <h2 id="allies-title">
            {t("allies.title")}
            <span className="count">{p.allies.length}</span>
          </h2>
          <span className="faint small">{t("peers.listSub", { n: p.peerMinMatches })}</span>
        </div>
        <DataTable
          spec={peerTable(ctx, {
            label: t("allies.title"),
            peers: p.allies,
            total,
            ownerId,
            friendMin: p.friendMinMatches,
            profiles,
          })}
        />
      </section>
      <section aria-labelledby="enemies-title">
        <div className="section-head">
          <h2 id="enemies-title">
            {t("enemies.title")}
            <span className="count">{p.enemies.length}</span>
          </h2>
          <span className="faint small">{t("peers.listSub", { n: p.peerMinMatches })}</span>
        </div>
        <DataTable spec={peerTable(ctx, { label: t("enemies.title"), peers: p.enemies, total, ownerId, profiles })} />
      </section>
    </>
  );
}
