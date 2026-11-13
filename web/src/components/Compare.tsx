// "Me vs them": two players side by side over their stored matches with
// metadata. Server components over lib/seen.ts; the better value of each
// metric is highlighted, colour stays for win/loss only.
import Link from "next/link";

import type { Profile } from "@/api/schema.gen.ts";
import type { I18n } from "@/i18n/core.ts";
import type { Lookup } from "@/lib/gamedata.ts";
import { type Bucket, winRate } from "@/lib/progress.ts";
import type { HeroLine, Shared, Summary } from "@/lib/seen.ts";

import { Avatar } from "./Avatar.tsx";
import { Icon } from "./Icon.tsx";

type Ctx = { i18n: I18n; look: Lookup };

export interface Side {
  id: string;
  name: string;
  profile?: Profile;
  summary: Summary;
  heroes: HeroLine[];
}

const pctOf = (i18n: I18n, b: Bucket) => (b.matches ? i18n.pct(b.wins, b.matches) : "—");

export function CompareHead({ i18n, a, b }: { i18n: I18n; a: Side; b: Side }) {
  const { t } = i18n;
  const card = (s: Side) => (
    <Link href={`/players/${s.id}`} className="vs-player">
      <Avatar src={s.profile?.avatarFull} name={s.name} size={64} />
      <span className="stack gap-4 vs-name">
        <span className="ellipsis vs-title">{s.name}</span>
        <span className="faint small">{t("compare.seen", { n: i18n.num(s.summary.matches) })}</span>
      </span>
    </Link>
  );
  return (
    <section className="vs-head" aria-label={t("compare.title")}>
      <span className="eyebrow">{t("compare.title")}</span>
      <div className="vs-row">
        {card(a)}
        <span className="vs-mark" aria-hidden="true">
          vs
        </span>
        {card(b)}
      </div>
    </section>
  );
}

export function SharedCard({ i18n, shared, a, b }: { i18n: I18n; shared: Shared; a: Side; b: Side }) {
  const { t } = i18n;
  if (shared.matches.length === 0) {
    return (
      <section aria-labelledby="shared-title">
        <h2 id="shared-title">{t("compare.shared")}</h2>
        <p className="muted">{t("compare.noShared")}</p>
      </section>
    );
  }
  const { together, against } = shared;
  return (
    <section aria-labelledby="shared-title">
      <h2 id="shared-title">
        {t("compare.shared")}
        <span className="count">{shared.matches.length}</span>
      </h2>
      <div className="form-tiles">
        <div className="form-tile">
          <span className="stat-label">{t("compare.together")}</span>
          <span className="stat-value">{pctOf(i18n, together)}</span>
          <span className="faint small">
            {together.matches
              ? t("compare.line", { wr: pctOf(i18n, together), n: i18n.num(together.matches) })
              : t("compare.never")}
          </span>
        </div>
        <div className="form-tile">
          <span className="stat-label">{t("compare.against")}</span>
          <span className="stat-value">
            {against.matches ? `${against.wins} : ${against.matches - against.wins}` : "—"}
          </span>
          <span className="faint small">
            {against.matches
              ? `${t("compare.againstLine", { name: a.name, wins: against.wins, n: against.matches })} · ${t(
                  "compare.againstLine",
                  { name: b.name, wins: against.matches - against.wins, n: against.matches },
                )}`
              : t("compare.never")}
          </span>
        </div>
      </div>
    </section>
  );
}

interface MetricRow {
  label: string;
  a: number;
  b: number;
  text: (v: number) => string;
  /** Which way is better; "none" for neutral values (match length). */
  better: "high" | "low" | "none";
}

export function MetricsTable({ i18n, a, b }: { i18n: I18n; a: Side; b: Side }) {
  const { t } = i18n;
  const A = a.summary;
  const B = b.summary;
  const num = (v: number) => i18n.num(Math.round(v));
  const dec = (d: number) => (v: number) => i18n.dec(v, d);
  const rows: MetricRow[] = [
    { label: t("stat.winRate"), a: winRate(A), b: winRate(B), text: (v) => i18n.pct(v, 1), better: "high" },
    { label: t("col.kda"), a: A.kda, b: B.kda, text: dec(2), better: "high" },
    { label: t("compare.kills"), a: A.kills, b: B.kills, text: dec(1), better: "high" },
    { label: t("metric.deaths"), a: A.deaths, b: B.deaths, text: dec(1), better: "low" },
    { label: t("compare.assists"), a: A.assists, b: B.assists, text: dec(1), better: "high" },
    { label: t("col.soulsPerMin"), a: A.soulsPerMin, b: B.soulsPerMin, text: num, better: "high" },
    { label: t("compare.damagePerMin"), a: A.damagePerMin, b: B.damagePerMin, text: num, better: "high" },
    { label: t("compare.healingPerMin"), a: A.healingPerMin, b: B.healingPerMin, text: num, better: "high" },
    { label: t("col.lastHits"), a: A.lastHits, b: B.lastHits, text: num, better: "high" },
    { label: t("compare.duration"), a: A.duration, b: B.duration, text: (v) => i18n.duration(Math.round(v)), better: "none" },
  ];
  const winner = (r: MetricRow): "a" | "b" | null => {
    if (r.better === "none" || !A.matches || !B.matches || r.text(r.a) === r.text(r.b)) return null;
    return (r.a > r.b) === (r.better === "high") ? "a" : "b";
  };
  return (
    <section aria-labelledby="metrics-title">
      <div className="section-head">
        <h2 id="metrics-title">{t("compare.metrics")}</h2>
        <span className="faint small">{t("compare.basis")}</span>
      </div>
      <div className="card vs-metrics" role="table" aria-label={t("compare.metrics")}>
        <div className="vs-metric vs-metric-head" role="row">
          <span role="columnheader" className="ellipsis">
            {a.name}
          </span>
          <span role="columnheader" />
          <span role="columnheader" className="ellipsis end">
            {b.name}
          </span>
        </div>
        {rows.map((r) => {
          const w = winner(r);
          const max = Math.max(r.a, r.b) || 1;
          return (
            <div key={r.label} className="vs-metric" role="row">
              <span role="cell" className={`mono vs-value${w === "a" ? " best" : ""}`}>
                {A.matches ? r.text(r.a) : "—"}
              </span>
              <span role="rowheader" className="vs-label">
                <span className="vs-bar left" aria-hidden="true">
                  <span style={{ width: `${(r.a / max) * 100}%` }} className={w === "a" ? "best" : undefined} />
                </span>
                <span className="small muted">{r.label}</span>
                <span className="vs-bar" aria-hidden="true">
                  <span style={{ width: `${(r.b / max) * 100}%` }} className={w === "b" ? "best" : undefined} />
                </span>
              </span>
              <span role="cell" className={`mono vs-value end${w === "b" ? " best" : ""}`}>
                {B.matches ? r.text(r.b) : "—"}
              </span>
            </div>
          );
        })}
      </div>
    </section>
  );
}

export function HeroesSideBySide({ i18n, look, a, b }: Ctx & { a: Side; b: Side }) {
  const { t } = i18n;
  const list = (s: Side) => (
    <div className="card stack">
      <span className="card-title ellipsis">{s.name}</span>
      <ul className="edges">
        {s.heroes.slice(0, 5).map((h) => (
          <li key={h.heroId} className="row">
            {look.heroIcon(h.heroId) && <Icon src={look.heroIcon(h.heroId)!} size={28} />}
            <span className="stack gap-4 edge-name">
              <span className="ellipsis">{look.heroName(h.heroId)}</span>
              <span className="faint small">{t("compare.line", { wr: pctOf(i18n, h), n: i18n.num(h.matches) })}</span>
            </span>
            <span className="mono small muted">{i18n.dec(h.kda, 2)}</span>
          </li>
        ))}
      </ul>
    </div>
  );
  return (
    <section aria-labelledby="vs-heroes-title">
      <h2 id="vs-heroes-title">{t("compare.heroes")}</h2>
      <div className="grid-2">
        {list(a)}
        {list(b)}
      </div>
    </section>
  );
}
