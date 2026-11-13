// Profile "Teammates" tab: party effect, friends, hero matchups. Server
// components over lib/peers.ts; colour only carries meaning (better / worse).
import Link from "next/link";

import type { Peer } from "@/api/schema.gen.ts";
import type { I18n } from "@/i18n/core.ts";
import type { Lookup } from "@/lib/gamedata.ts";
import { pp, signedPp } from "@/lib/peerViews.ts";
import {
  type HeroEdge,
  type Party,
  MIN_SAMPLE,
  NOTABLE,
  biggestImpact,
  heroEdges,
  weakestParty,
  withWithout,
} from "@/lib/peers.ts";
import { type Bucket, MIN_BUCKET, winRate } from "@/lib/progress.ts";

import { Icon } from "./Icon.tsx";

type Ctx = { i18n: I18n; look: Lookup };

const pctOf = (i18n: I18n, share: number) => i18n.pct(share, 1);

const toneOf = (d: number | null) =>
  d === null ? "faint" : d >= NOTABLE ? "tone-win" : d <= -NOTABLE ? "tone-loss" : "faint";

const playerName = (i18n: I18n, id: string) => i18n.t("player.title", { id });

function HeroIcon({ look, heroId, size }: { look: Lookup; heroId: number; size: number }) {
  const src = look.heroIcon(heroId);
  return src ? <Icon src={src} size={size} /> : null;
}

// ---------- party ----------

export function PartySection({
  i18n,
  party,
  friendMin,
  sub,
}: {
  i18n: I18n;
  party: Party;
  friendMin: number;
  sub: string;
}) {
  const { t } = i18n;
  const { solo, party: withF, bySize } = party;
  const fair = solo.matches >= MIN_BUCKET && withF.matches >= MIN_BUCKET;
  const delta = fair ? winRate(withF) - winRate(solo) : null;
  const weak = weakestParty(party, MIN_BUCKET);
  const sizeLabel = (n: number) => (n === 0 ? t("party.size0") : t("party.sizeN", { n }));
  const tile = (label: string, b: Bucket) => (
    <div className="form-tile">
      <span className="stat-label">{label}</span>
      <span className="stat-value">{b.matches ? pctOf(i18n, winRate(b)) : "—"}</span>
      <span className="faint small">{t("party.matches", { n: i18n.num(b.matches) })}</span>
    </div>
  );
  return (
    <section aria-labelledby="party-title">
      <div className="section-head">
        <h2 id="party-title">{t("party.title")}</h2>
        <span className="faint small">{sub}</span>
      </div>
      <div className="grid-2">
        <div className="card stack gap-16">
          <div className="form-tiles">
            {tile(t("party.solo"), solo)}
            {tile(t("party.with"), withF)}
          </div>
          <p className="small">
            {delta === null ? (
              <span className="muted">{t("party.same")}</span>
            ) : (
              <span className={toneOf(delta)}>
                {Math.abs(delta) < NOTABLE
                  ? t("party.same")
                  : t(delta > 0 ? "party.better" : "party.worse", { pp: pp(i18n, delta) })}
              </span>
            )}
          </p>
          {weak && (
            <p className="small tone-loss">
              {t("party.weakSize", {
                n: weak.friends,
                wr: pctOf(i18n, winRate(weak.bucket)),
                m: i18n.num(weak.bucket.matches),
              })}
            </p>
          )}
          <p className="faint small">{t("party.hint", { n: friendMin })}</p>
        </div>
        <div className="card stack party-chart">
          <div className="col-plot" role="img" aria-label={t("party.chartLabel")}>
            <div className="hline" style={{ bottom: "50%" }} />
            {bySize.map((b, n) => (
              <div key={n} className="col" title={t("party.tip", { size: sizeLabel(n), n: b.matches, wr: pctOf(i18n, winRate(b)) })}>
                <span className={`mono small ${b.matches < MIN_BUCKET ? "faint" : "muted"}`}>
                  {b.matches ? pctOf(i18n, winRate(b)) : "—"}
                </span>
                <span
                  className={`col-bar${b.matches < MIN_BUCKET ? " few" : ""}${b.matches >= MIN_BUCKET && winRate(b) <= winRate(solo) - NOTABLE ? " down" : ""}`}
                  style={{ height: `${winRate(b) * 100}%` }}
                />
              </div>
            ))}
          </div>
          <div className="axis">
            {bySize.map((b, n) => (
              <span key={n}>
                {sizeLabel(n)} · {b.matches}
              </span>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}

// ---------- friends ----------

export function FriendsSection({
  i18n,
  look,
  friends,
  total,
  ownerId,
  friendMin,
}: Ctx & { friends: Peer[]; total: Bucket; ownerId: string; friendMin: number }) {
  const { t } = i18n;
  if (friends.length === 0) {
    return (
      <section aria-labelledby="friends-title">
        <h2 id="friends-title">{t("friends.title")}</h2>
        <p className="muted">{t("friends.empty", { n: friendMin })}</p>
      </section>
    );
  }
  const impact = biggestImpact(friends, total);
  return (
    <section aria-labelledby="friends-title">
      <div className="section-head">
        <h2 id="friends-title">
          {t("friends.title")}
          <span className="count">{friends.length}</span>
        </h2>
      </div>
      <p className="small">
        {impact ? (
          <span className={toneOf(impact.delta)}>
            {t(impact.delta > 0 ? "friends.impactUp" : "friends.impactDown", {
              name: playerName(i18n, impact.peer.accountId),
              pp: pp(i18n, impact.delta),
            })}
          </span>
        ) : (
          <span className="muted">{t("friends.noImpact")}</span>
        )}
      </p>
      <div className="friends">
        {friends.map((f) => {
          const ww = withWithout(f, total);
          return (
            <article key={f.accountId} className="friend" data-testid="friend">
              <div className="row friend-head">
                <HeroIcon look={look} heroId={f.topHeroId} size={40} />
                <div className="stack gap-4 friend-name">
                  <Link href={`/players/${f.accountId}`} className="ellipsis">
                    {playerName(i18n, f.accountId)}
                  </Link>
                  <span className="faint small ellipsis">
                    {t("friends.topHero", { hero: look.heroName(f.topHeroId) })}
                  </span>
                </div>
              </div>
              <div className="row between friend-stats">
                <span className="stack gap-4">
                  <span className="stat-label">{t("friends.wrWith")}</span>
                  <span className="stat-value">{pctOf(i18n, winRate(f))}</span>
                </span>
                <span className="stack gap-4 end">
                  <span className="stat-label">{t("friends.together")}</span>
                  <span className="stat-value">{i18n.num(f.matches)}</span>
                </span>
              </div>
              <span className={`small ${f.matches >= MIN_SAMPLE ? toneOf(ww.delta) : "faint"}`}>
                {ww.delta === null ? "—" : signedPp(i18n, ww.delta)}{" "}
                <span className="faint">· {t("friends.others", { wr: pctOf(i18n, winRate(ww.without)) })}</span>
              </span>
              <Link href={`/matches/${f.lastMatchId}/players/${ownerId}`} className="faint small">
                {t("friends.last", { date: i18n.date(f.lastPlayed).split(",")[0] ?? "" })}
              </Link>
            </article>
          );
        })}
      </div>
    </section>
  );
}

// ---------- hero matchups ----------

function EdgeList({ i18n, look, title, edges }: Ctx & { title: string; edges: HeroEdge[] }) {
  const { t } = i18n;
  return (
    <div className="card stack">
      <span className="card-title">{title}</span>
      {edges.length === 0 ? (
        <span className="faint small">{t("matchups.none")}</span>
      ) : (
        <ul className="edges">
          {edges.map((e) => (
            <li key={e.heroId} className="row">
              <HeroIcon look={look} heroId={e.heroId} size={28} />
              <span className="stack gap-4 edge-name">
                <span className="ellipsis">{look.heroName(e.heroId)}</span>
                <span className="faint small">
                  {t("matchups.row", { wr: pctOf(i18n, winRate(e)), n: i18n.num(e.matches) })}
                </span>
              </span>
              <span className={`mono small ${toneOf(e.delta)}`}>{signedPp(i18n, e.delta)}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

export function MatchupEdges({
  i18n,
  look,
  edges,
  total,
  min,
}: Ctx & { edges: ReturnType<typeof heroEdges>; total: Bucket; min: number }) {
  const { t } = i18n;
  return (
    <>
      <div className="edge-grid">
        <EdgeList i18n={i18n} look={look} title={t("matchups.hardest")} edges={edges.hardest} />
        <EdgeList i18n={i18n} look={look} title={t("matchups.easiest")} edges={edges.easiest} />
        <EdgeList i18n={i18n} look={look} title={t("matchups.bestAllies")} edges={edges.bestAllies} />
      </div>
      <p className="faint small">{t("matchups.hint", { n: min, wr: pctOf(i18n, winRate(total)) })}</p>
    </>
  );
}
