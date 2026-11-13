// Profile "Progress" tab: form, monthly trend, lobby level, development
// speed, sessions and records. Server-rendered HTML and SVG over the results
// of lib/progress.ts; colour only carries meaning (better / worse).
import Link from "next/link";

import type { I18n } from "@/i18n/core.ts";
import { duration } from "@/lib/format.ts";
import type { Lookup } from "@/lib/gamedata.ts";
import {
  type Agg,
  type Bucket,
  type Form,
  type Month,
  type Records,
  type Speed,
  MIN_BUCKET,
  badgeRank,
  badgeStep,
  minHeroMatches,
  weakHours,
  weakestSessionPos,
  winRate,
} from "@/lib/progress.ts";

import { Icon } from "./Icon.tsx";

type Ctx = { i18n: I18n; look: Lookup };

export const TREND_METRICS = ["spm", "kda", "deaths"] as const;
export type TrendMetric = (typeof TREND_METRICS)[number];

const metricValue: Record<TrendMetric, (a: Agg) => number> = {
  spm: (a) => a.soulsPerMin,
  kda: (a) => a.kda,
  deaths: (a) => a.deaths,
};

function fmtMetric(i18n: I18n, m: TrendMetric, v: number): string {
  return m === "spm" ? i18n.num(Math.round(v)) : i18n.dec(v, m === "kda" ? 2 : 1);
}

const pctOf = (i18n: I18n, share: number) => i18n.pct(share, 1);

/** "+10.0 pp" / "−3.6%" with the sign always shown. */
function signed(i18n: I18n, n: number, digits: number): string {
  const s = i18n.dec(Math.abs(n), digits);
  return n > 0 ? `+${s}` : n < 0 ? `−${s}` : s;
}

type Tone = "good" | "bad" | "same";
const toneClass: Record<Tone, string> = { good: "tone-win", bad: "tone-loss", same: "faint" };

/** Better / worse / about the same, for a metric where larger is better (or not). */
function tone(delta: number, threshold: number, largerIsBetter = true): Tone {
  if (Math.abs(delta) < threshold) return "same";
  return delta > 0 === largerIsBetter ? "good" : "bad";
}

export function rankName(i18n: I18n, badge: number): string {
  const r = badgeRank(badge);
  if (!r) return "—";
  const tier = r.tier >= 1 && r.tier <= 11 ? i18n.t(`rank.${r.tier}` as "rank.1") : String(r.tier);
  return i18n.t("rank.badge", { tier, sub: r.sub });
}

const monthLabel = (m: Month) => `${String(m.month).padStart(2, "0")}.${String(m.year % 100).padStart(2, "0")}`;

function HeroIcon({ look, heroId, size }: { look: Lookup; heroId: number; size: number }) {
  const src = look.heroIcon(heroId);
  return src ? <Icon src={src} size={size} /> : null;
}

// ---------- form ----------

export function FormCard({ i18n, form }: { i18n: I18n; form: Form | null }) {
  const { t } = i18n;
  if (!form) {
    return (
      <section aria-labelledby="form-title">
        <h2 id="form-title">{t("form.title")}</h2>
        <p className="muted">{t("form.tooFew")}</p>
      </section>
    );
  }
  const { now, was } = form;
  const dWr = (now.winRate - was.winRate) * 100;
  const dSpm = was.soulsPerMin ? (now.soulsPerMin / was.soulsPerMin - 1) * 100 : 0;
  const tiles: { label: string; now: string; was: string; delta: string; tone: Tone }[] = [
    {
      label: t("stat.winRate"),
      now: pctOf(i18n, now.winRate),
      was: pctOf(i18n, was.winRate),
      delta: t("unit.pp", { n: signed(i18n, dWr, 1) }),
      tone: tone(dWr, 2),
    },
    {
      label: t("col.soulsPerMin"),
      now: i18n.num(Math.round(now.soulsPerMin)),
      was: i18n.num(Math.round(was.soulsPerMin)),
      delta: `${signed(i18n, dSpm, 1)}%`,
      tone: tone(dSpm, 2),
    },
    {
      label: t("col.kda"),
      now: i18n.dec(now.kda, 2),
      was: i18n.dec(was.kda, 2),
      delta: signed(i18n, now.kda - was.kda, 2),
      tone: tone(now.kda - was.kda, 0.1),
    },
    {
      label: t("metric.deaths"),
      now: i18n.dec(now.deaths, 1),
      was: i18n.dec(was.deaths, 1),
      delta: signed(i18n, now.deaths - was.deaths, 1),
      tone: tone(now.deaths - was.deaths, 0.5, false),
    },
  ];
  return (
    <section aria-labelledby="form-title">
      <div className="section-head">
        <h2 id="form-title">{t("form.title")}</h2>
        <span className="faint small">{t("form.sub", { n: form.size })}</span>
      </div>
      <div className={`card form-card form-${form.trend}`}>
        <p className="form-line">
          <span className={`pill pill-${form.trend}`}>
            <TrendGlyph trend={form.trend} />
            {t(`form.${form.trend}`)}
          </span>
          <span>
            {t("form.line", {
              wr: t("unit.pp", { n: signed(i18n, dWr, 1) }),
              spm: `${signed(i18n, dSpm, 1)}%`,
            })}
          </span>
        </p>
        <div className="form-tiles">
          {tiles.map((x) => (
            <div key={x.label} className="form-tile">
              <span className="stat-label">{x.label}</span>
              <span className="row">
                <span className="stat-value">{x.now}</span>
                <span className={`mono small ${toneClass[x.tone]}`}>{x.delta}</span>
              </span>
              <span className="faint small">{t("form.was", { v: x.was })}</span>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

function TrendGlyph({ trend }: { trend: Form["trend"] }) {
  const d = trend === "up" ? "M3 17l6-6 4 4 8-8M15 7h6v6" : trend === "down" ? "M3 7l6 6 4-4 8 8M15 17h6v-6" : "M3 12h18";
  return (
    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" aria-hidden="true">
      <path d={d} />
    </svg>
  );
}

// ---------- monthly trend ----------

/** Months with fewer matches are drawn faded: too few to trust. */
const FEW_MATCHES = 5;

export function TrendChart({
  i18n,
  months,
  metric,
  hrefFor,
}: {
  i18n: I18n;
  months: Month[];
  metric: TrendMetric;
  hrefFor: (m: TrendMetric) => string;
}) {
  const { t } = i18n;
  const metricLabel = { spm: t("col.soulsPerMin"), kda: t("col.kda"), deaths: t("metric.deaths") }[metric];
  const vals = months.map((m) => metricValue[metric](m.agg));
  const lo = Math.min(...vals);
  const hi = Math.max(...vals);
  const pad = (hi - lo) * 0.15 || Math.max(hi * 0.1, 1);
  const y = (v: number) => 240 - ((v - (lo - pad)) / (hi - lo + 2 * pad)) * 240;
  const x = (i: number) => ((i + 0.5) / months.length) * 1000;
  const line = vals.map((v, i) => `${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(" ");
  const first = months[0];
  const last = months.at(-1);

  return (
    <section aria-labelledby="trend-title">
      <div className="section-head">
        <h2 id="trend-title">{t("trend.title")}</h2>
        <nav className="segmented" aria-label={t("trend.metric")}>
          {TREND_METRICS.map((m) => (
            <Link key={m} href={hrefFor(m)} aria-current={m === metric ? "true" : undefined} scroll={false}>
              {t(`trend.${m}`)}
            </Link>
          ))}
        </nav>
      </div>
      <div className="card chart-card">
        <div className="legend">
          <span>
            <i style={{ background: "var(--text)" }} />
            {metricLabel}
          </span>
          <span>
            <i className="swatch" style={{ background: "var(--surface-3)" }} />
            {t("trend.bars")}
          </span>
        </div>
        <div className="trend-plot">
          <div className="hline" style={{ top: "50%" }} />
          <div className="trend-bars">
            {months.map((m, i) => (
              <div
                key={m.key}
                className={`trend-bar${m.agg.winRate >= 0.5 ? " up" : ""}${m.agg.matches < FEW_MATCHES ? " few" : ""}`}
                style={{ height: `${Math.max(m.agg.winRate * 100, 1)}%` }}
                title={t("trend.tip", {
                  month: monthLabel(m),
                  matches: m.agg.matches,
                  wr: pctOf(i18n, m.agg.winRate),
                  metric: metricLabel,
                  value: fmtMetric(i18n, metric, vals[i]!),
                })}
              />
            ))}
          </div>
          <svg
            className="overlay"
            viewBox="0 0 1000 240"
            preserveAspectRatio="none"
            role="img"
            aria-label={t("trend.chartLabel", {
              metric: metricLabel,
              first: first ? `${fmtMetric(i18n, metric, vals[0]!)} (${monthLabel(first)})` : "—",
              last: last ? `${fmtMetric(i18n, metric, vals.at(-1)!)} (${monthLabel(last)})` : "—",
            })}
          >
            <polyline points={line} fill="none" stroke="var(--text)" strokeWidth="2" vectorEffect="non-scaling-stroke" strokeLinejoin="round" />
          </svg>
        </div>
        <div className="axis">
          {months.map((m) => (
            <span key={m.key}>{monthLabel(m)}</span>
          ))}
        </div>
      </div>
    </section>
  );
}

// ---------- lobby level ----------

export function LobbyChart({ i18n, months }: { i18n: I18n; months: Month[] }) {
  const { t } = i18n;
  const ms = months.filter((m) => m.badge > 0);
  if (ms.length === 0) {
    return (
      <section aria-labelledby="lobby-title">
        <h2 id="lobby-title">{t("lobby.title")}</h2>
        <p className="muted">{t("lobby.empty")}</p>
      </section>
    );
  }
  const steps = ms.map((m) => badgeStep(m.badge)!);
  const lo = Math.max(0, Math.floor((Math.min(...steps) - 2) / 6) * 6);
  const hi = Math.ceil((Math.max(...steps) + 2) / 6) * 6;
  const top = (s: number) => (1 - (s - lo) / (hi - lo)) * 100; // % from the top
  const x = (i: number) => (ms.length === 1 ? 50 : (i / (ms.length - 1)) * 100);
  const line = steps.map((s, i) => `${(x(i) * 10).toFixed(1)},${(top(s) * 2).toFixed(1)}`).join(" ");
  const tiers = Array.from({ length: (hi - lo) / 6 + 1 }, (_, i) => lo + i * 6);
  const latest = ms.at(-1)!;
  const peak = ms.reduce((a, b) => (badgeStep(b.badge)! > badgeStep(a.badge)! ? b : a));
  const change = steps.at(-1)! - steps[0]!;

  return (
    <section aria-labelledby="lobby-title">
      <div className="section-head">
        <h2 id="lobby-title">{t("lobby.title")}</h2>
        <span className="faint small">{t("lobby.sub")}</span>
      </div>
      <div className="card chart-card">
        <div className="lobby-facts">
          <div className="form-tile">
            <span className="stat-label">{t("lobby.latest")}</span>
            <span className="stat-value">{rankName(i18n, latest.badge)}</span>
            <span className="faint small">{monthLabel(latest)}</span>
          </div>
          <div className="form-tile">
            <span className="stat-label">{t("lobby.peak")}</span>
            <span className="stat-value">{rankName(i18n, peak.badge)}</span>
            <span className="faint small">{monthLabel(peak)}</span>
          </div>
          <div className="form-tile">
            <span className="stat-label">{t("lobby.change", { n: ms.length })}</span>
            <span className={`stat-value ${toneClass[tone(change, 1)]}`}>{signed(i18n, change, 0)}</span>
            <span className="faint small">{t("lobby.steps", { n: Math.abs(change) })}</span>
          </div>
        </div>
        <div className="lobby-plot">
          {tiers.map((s) => (
            <div key={s} className="tier-line" style={{ top: `${top(s)}%` }}>
              <span>{s / 6 + 1 <= 11 ? t(`rank.${s / 6 + 1}` as "rank.1") : ""}</span>
            </div>
          ))}
          <svg
            className="overlay"
            viewBox="0 0 1000 200"
            preserveAspectRatio="none"
            role="img"
            aria-label={t("lobby.chartLabel", {
              first: `${rankName(i18n, ms[0]!.badge)} (${monthLabel(ms[0]!)})`,
              last: `${rankName(i18n, latest.badge)} (${monthLabel(latest)})`,
            })}
          >
            <polyline points={line} fill="none" stroke="var(--sapphire)" strokeWidth="2" vectorEffect="non-scaling-stroke" strokeLinejoin="round" />
          </svg>
          {ms.map((m, i) => (
            <span
              key={m.key}
              className="dot"
              style={{ left: `${x(i)}%`, top: `${top(steps[i]!)}%` }}
              title={`${monthLabel(m)}: ${rankName(i18n, m.badge)}`}
            />
          ))}
        </div>
        <div className="axis spread">
          {ms.map((m) => (
            <span key={m.key}>{monthLabel(m)}</span>
          ))}
        </div>
      </div>
    </section>
  );
}

// ---------- development speed ----------

export function SpeedSection({ i18n, look, speed, accountId }: Ctx & { speed: Speed; accountId: number }) {
  const { t } = i18n;
  if (speed.matches === 0) {
    return (
      <section aria-labelledby="speed-title">
        <h2 id="speed-title">{t("speed.title")}</h2>
        <p className="muted">{t("speed.empty")}</p>
      </section>
    );
  }
  return (
    <section aria-labelledby="speed-title">
      <div className="section-head">
        <h2 id="speed-title">{t("speed.title")}</h2>
        <span className="faint small">{t("speed.sub", { n: i18n.num(speed.matches) })}</span>
      </div>
      <div className="grid-2">
        <SoulCurve i18n={i18n} speed={speed} />
        <div className="stack gap-16">
          <Targets i18n={i18n} speed={speed} />
          <Sources i18n={i18n} speed={speed} />
        </div>
      </div>
      <FastHeroes i18n={i18n} look={look} speed={speed} accountId={accountId} />
    </section>
  );
}

function SoulCurve({ i18n, speed }: { i18n: I18n; speed: Speed }) {
  const { t } = i18n;
  const marks = speed.marks.filter((m) => m.matches > 0);
  const maxT = Math.max(...marks.map((m) => m.t), 1);
  const maxV = Math.max(...marks.map((m) => m.top), 1) * 1.08;
  const x = (s: number) => (s / maxT) * 400;
  const y = (v: number) => 170 - (v / maxV) * 160;
  const pts = (k: "avg" | "top") => [`0,170`, ...marks.map((m) => `${x(m.t).toFixed(1)},${y(m[k]).toFixed(1)}`)].join(" ");
  const area =
    `M0,170 ` +
    marks.map((m) => `L${x(m.t).toFixed(1)},${y(m.top).toFixed(1)}`).join(" ") +
    " " +
    [...marks]
      .reverse()
      .map((m) => `L${x(m.t).toFixed(1)},${y(m.avg).toFixed(1)}`)
      .join(" ") +
    " Z";
  const values = marks.map((m) => `${t("speed.min", { n: m.t / 60 })}: ${i18n.num(Math.round(m.avg))}`).join(", ");
  return (
    <div className="card stack">
      <div className="section-head">
        <span className="card-title">{t("speed.curve")}</span>
        <div className="legend">
          <span>
            <i style={{ background: "var(--text)" }} />
            {t("speed.avg")}
          </span>
          <span>
            <i className="dashed" />
            {t("speed.top")}
          </span>
        </div>
      </div>
      <svg className="chart" viewBox="0 0 400 180" role="img" aria-label={t("speed.curveLabel", { values })}>
        <line className="grid" x1="0" x2="400" y1="170" y2="170" />
        {marks.map((m) => (
          <line key={m.t} className="grid faint-grid" x1={x(m.t)} x2={x(m.t)} y1="10" y2="170" />
        ))}
        <path d={area} fill="rgba(237,237,237,0.07)" />
        <polyline points={pts("top")} fill="none" stroke="var(--faint)" strokeWidth="1.5" strokeDasharray="4 4" />
        <polyline points={pts("avg")} fill="none" stroke="var(--text)" strokeWidth="2" strokeLinejoin="round" />
        {marks.map((m) => (
          <circle key={m.t} cx={x(m.t)} cy={y(m.avg)} r="3.5" fill="var(--text)" />
        ))}
      </svg>
      <div className="marks">
        {marks.map((m) => (
          <div key={m.t}>
            <span className="faint">{t("speed.min", { n: m.t / 60 })}</span>
            <span className="mono">{i18n.num(Math.round(m.avg))}</span>
            <span className="mono faint">{i18n.num(Math.round(m.top))}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function Targets({ i18n, speed }: { i18n: I18n; speed: Speed }) {
  const { t } = i18n;
  const ts = speed.targets.filter((x) => x.avg > 0);
  const max = Math.max(...ts.map((x) => x.avg), 1) * 1.15;
  return (
    <div className="card stack">
      <span className="card-title">{t("speed.targets")}</span>
      {ts.map((x) => (
        <div key={x.souls} className="target-row">
          <span className="mono muted">{i18n.num(x.souls / 1000)}k</span>
          <span className="target-track" title={t("speed.reached", { pct: pctOf(i18n, x.reached) })}>
            <span className="fast" style={{ width: `${(x.fast / max) * 100}%` }} />
            <span className="avg" style={{ width: `${(x.avg / max) * 100}%` }} />
          </span>
          <span className="mono">{duration(x.avg)}</span>
        </div>
      ))}
      <span className="faint small">{t("speed.targetsHint")}</span>
    </div>
  );
}

const SOURCE_COLOR: Record<string, string> = {
  laneCreeps: "var(--text)",
  heroes: "var(--amber)",
  neutrals: "var(--faint)",
  bosses: "var(--sapphire)",
  treasure: "var(--border-strong)",
};

function Sources({ i18n, speed }: { i18n: I18n; speed: Speed }) {
  const { t } = i18n;
  return (
    <div className="card stack">
      <span className="card-title">{t("speed.sources")}</span>
      <div className="stacked" aria-hidden="true">
        {speed.sources.map((s) => (
          <span key={s.key} style={{ width: `${s.share * 100}%`, background: SOURCE_COLOR[s.key] }} />
        ))}
      </div>
      <ul className="sources">
        {speed.sources.map((s) => (
          <li key={s.key}>
            <i style={{ background: SOURCE_COLOR[s.key] }} />
            <span className="muted">{t(`source.${s.key}`)}</span>
            <span className="mono">{pctOf(i18n, s.share)}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

const TOP_HEROES = 6;

function FastHeroes({ i18n, look, speed, accountId }: Ctx & { speed: Speed; accountId: number }) {
  const { t } = i18n;
  const hs = speed.heroes.slice(0, TOP_HEROES);
  const max = Math.max(...hs.map((h) => h.souls), 1);
  const min = Math.min(...hs.map((h) => h.souls), max) * 0.9;
  return (
    <div className="card stack">
      <div className="section-head">
        <span className="card-title">{t("speed.heroes")}</span>
        <span className="faint small">
          {t("speed.heroesHint", { time: duration(speed.heroMark), n: minHeroMatches(speed.matches) })}
        </span>
      </div>
      {hs.length === 0 ? (
        <p className="muted">{t("speed.heroesEmpty")}</p>
      ) : (
        <ul className="fast-heroes">
          {hs.map((h) => (
            <li key={h.heroId}>
              <HeroIcon look={look} heroId={h.heroId} size={36} />
              <div className="stack gap-4">
                <span className="row between">
                  <Link href={`/players/${accountId}/matches?hero=${h.heroId}`}>{look.heroName(h.heroId)}</Link>
                  <span className="mono small">
                    {i18n.num(Math.round(h.souls))} <span className="faint">· {t("speed.heroMatches", { n: h.matches })}</span>
                  </span>
                </span>
                <span className="bar">
                  <span style={{ width: `${Math.max(4, ((h.souls - min) / (max - min || 1)) * 100)}%` }} />
                </span>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// ---------- sessions and time ----------

export function SessionsSection({
  i18n,
  sessions,
  hours,
  afterLosses,
  overall,
  tz,
}: {
  i18n: I18n;
  sessions: Bucket[];
  hours: Bucket[];
  afterLosses: Bucket;
  overall: number;
  tz: string;
}) {
  const { t } = i18n;
  const posLabel = (i: number) =>
    i === sessions.length - 1 ? t("sessions.posLast", { n: i + 1 }) : t("sessions.pos", { n: i + 1 });
  const weak = weakestSessionPos(sessions);
  const tilt = afterLosses.matches >= MIN_BUCKET && winRate(afterLosses) <= overall - 0.05;
  const peak = hours.reduce((best, b, h) => (b.matches > hours[best]!.matches ? h : best), 0);
  const weakH = weakHours(hours, overall);
  const maxH = Math.max(...hours.map((b) => b.matches), 1);
  const hourTone = (b: Bucket) =>
    b.matches < 10 ? "" : winRate(b) >= overall + 0.05 ? " up" : winRate(b) <= overall - 0.08 ? " down" : "";

  return (
    <section aria-labelledby="sessions-title">
      <h2 id="sessions-title">{t("sessions.title")}</h2>
      <div className="grid-2">
        <div className="card stack">
          <span className="card-title">{t("sessions.byPos")}</span>
          <div className="col-plot" role="img" aria-label={t("sessions.chartLabel")}>
            <div className="hline" style={{ bottom: "50%" }} />
            {sessions.map((b, i) => (
              <div
                key={i}
                className="col"
                title={t("sessions.posTip", { pos: posLabel(i), n: b.matches, wr: pctOf(i18n, winRate(b)) })}
              >
                <span className={`mono small ${i === weak ? "tone-loss" : b.matches < MIN_BUCKET ? "faint" : "muted"}`}>
                  {b.matches ? pctOf(i18n, winRate(b)) : "—"}
                </span>
                <span
                  className={`col-bar${i === weak ? " down" : ""}${b.matches < MIN_BUCKET ? " few" : ""}`}
                  style={{ height: `${winRate(b) * 100}%` }}
                />
              </div>
            ))}
          </div>
          <div className="axis">
            {sessions.map((b, i) => (
              <span key={i}>
                {posLabel(i)} · {b.matches}
              </span>
            ))}
          </div>
          <p className="small muted">
            {weak !== null
              ? t("sessions.weak", {
                  pos: posLabel(weak),
                  wr: pctOf(i18n, winRate(sessions[weak]!)),
                  first: pctOf(i18n, winRate(sessions[0]!)),
                })
              : t("sessions.noWeak")}{" "}
            {afterLosses.matches >= MIN_BUCKET &&
              t(tilt ? "sessions.tiltYes" : "sessions.tiltNo", { wr: pctOf(i18n, winRate(afterLosses)) })}
          </p>
        </div>
        <div className="card stack">
          <div className="section-head">
            <span className="card-title">{t("sessions.hours")}</span>
            <span className="faint small">{t("sessions.hoursHint", { tz })}</span>
          </div>
          <div className="hours-plot" role="img" aria-label={t("sessions.hoursLabel")}>
            {hours.map((b, h) => (
              <span
                key={h}
                className={`hour-bar${hourTone(b)}`}
                style={{ height: `${b.matches ? Math.max(3, (b.matches / maxH) * 100) : 0}%` }}
                title={t("sessions.hourTip", { hour: h, n: b.matches, wr: pctOf(i18n, winRate(b)) })}
              />
            ))}
          </div>
          <div className="axis hours-axis">
            {hours.map((_, h) => (
              <span key={h}>{h % 3 === 0 ? h : ""}</span>
            ))}
          </div>
          <p className="small muted">
            {hours[peak]!.matches > 0 &&
              t(weakH.includes(peak) ? "sessions.peakWeak" : "sessions.peak", {
                hour: peak,
                n: hours[peak]!.matches,
                wr: pctOf(i18n, winRate(hours[peak]!)),
              })}{" "}
            {weakH.some((h) => h !== peak) &&
              t("sessions.weakHours", {
                list: weakH
                  .filter((h) => h !== peak)
                  .map((h) => `${h}:00 (${pctOf(i18n, winRate(hours[h]!))})`)
                  .join(", "),
              })}
          </p>
        </div>
      </div>
    </section>
  );
}

// ---------- records ----------

export function RecordsSection({ i18n, look, records, accountId }: Ctx & { records: Records; accountId: number }) {
  const { t } = i18n;
  if (records.best.length === 0) return null;
  return (
    <section aria-labelledby="records-title">
      <h2 id="records-title">{t("records.title")}</h2>
      <div className="records">
        {records.best.map((b) => (
          <Link key={b.key} href={`/matches/${b.matchId}/players/${accountId}`} className="record">
            <span className="stat-label">{t(`record.${b.key}`)}</span>
            <span className="stat-value">{i18n.num(b.value)}</span>
            <span className="row faint small">
              <HeroIcon look={look} heroId={b.heroId} size={18} />
              <span className="ellipsis">
                {look.heroName(b.heroId)} · {i18n.date(b.startTime).split(",")[0]}
              </span>
            </span>
          </Link>
        ))}
        <div className="record">
          <span className="stat-label">{t("record.winStreak")}</span>
          <span className="stat-value tone-win">{records.winStreak}</span>
          <span className="faint small">{t("record.inRow")}</span>
        </div>
        <div className="record">
          <span className="stat-label">{t("record.lossStreak")}</span>
          <span className="stat-value tone-loss">{records.lossStreak}</span>
          <span className="faint small">{t("record.inRow")}</span>
        </div>
      </div>
    </section>
  );
}
