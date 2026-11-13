// Profile of a player we don't track but have seen in stored matches: what
// their lines in those matches tell, a sync button to load the full history
// and a shortcut to compare with the owner.
import Link from "next/link";

import type { Profile, SeenMatch } from "@/api/schema.gen.ts";
import type { SyncState } from "@/app/players/[id]/actions.ts";
import type { I18n } from "@/i18n/core.ts";
import { compareHref, seenHeroTable } from "@/lib/compareViews.ts";
import type { Lookup } from "@/lib/gamedata.ts";
import { winRate } from "@/lib/progress.ts";
import { heroLines, summarize } from "@/lib/seen.ts";

import { Avatar } from "./Avatar.tsx";
import { DataTable } from "./DataTable.tsx";
import { SyncButton } from "./SyncButton.tsx";

export function KnownPlayer({
  i18n,
  look,
  id,
  profile,
  matches,
  owner,
  action,
}: {
  i18n: I18n;
  look: Lookup;
  id: string;
  profile?: Profile;
  matches: SeenMatch[];
  /** The owner's account id, for the compare shortcut; null = none. */
  owner: string | null;
  action: (prev: SyncState | null) => Promise<SyncState>;
}) {
  const { t } = i18n;
  const name = profile?.personaName.trim() || t("player.title", { id });
  const s = summarize(matches);
  const tiles = [
    { label: t("stat.winRate"), value: i18n.pct(winRate(s), 1), hint: t("party.matches", { n: i18n.num(s.matches) }) },
    { label: t("col.kda"), value: i18n.dec(s.kda, 2), hint: `${i18n.dec(s.kills, 1)} / ${i18n.dec(s.deaths, 1)} / ${i18n.dec(s.assists, 1)}` },
    { label: t("col.soulsPerMin"), value: i18n.num(Math.round(s.soulsPerMin)), hint: "" },
    { label: t("compare.damagePerMin"), value: i18n.num(Math.round(s.damagePerMin)), hint: "" },
  ];
  return (
    <>
      <section className="profile-head">
        <div className="head-row">
          <div className="row player-id">
            <Avatar src={profile?.avatarFull} name={name} size={72} />
            <div className="stack">
              <span className="eyebrow">
                {t("player.account", { id })}
                {profile?.profileUrl && (
                  <>
                    {" · "}
                    <a href={profile.profileUrl} target="_blank" rel="noopener noreferrer">
                      {t("player.steamProfile")}
                    </a>
                  </>
                )}
              </span>
              <h1>{name}</h1>
              <span className="muted">{t("known.note", { n: i18n.num(s.matches) })}</span>
              <span className="faint small">{t("known.syncHint")}</span>
            </div>
          </div>
          <div className="row head-actions">
            {owner && owner !== id && (
              <Link href={compareHref(owner, id)} className="btn">
                {t("compare.withMe")}
              </Link>
            )}
            <SyncButton action={action} />
          </div>
        </div>
      </section>
      <section aria-label={t("compare.metrics")}>
        <div className="form-tiles">
          {tiles.map((x) => (
            <div key={x.label} className="form-tile">
              <span className="stat-label">{x.label}</span>
              <span className="stat-value">{x.value}</span>
              {x.hint && <span className="faint small">{x.hint}</span>}
            </div>
          ))}
        </div>
      </section>
      <section aria-labelledby="known-heroes-title">
        <h2 id="known-heroes-title">{t("heroes.title")}</h2>
        <DataTable spec={seenHeroTable({ i18n, look }, heroLines(matches))} />
      </section>
    </>
  );
}
