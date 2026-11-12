import type { SlotTimeline } from "@/api/schema.gen.ts";
import type { I18n } from "@/i18n/core.ts";
import { duration } from "@/lib/format.ts";

const W = 1072;
const H = 260;
const PAD = { top: 12, right: 12, bottom: 26, left: 48 };

/** Rounds a value up to a readable axis step: 1, 2, 5 × 10^n. */
function niceStep(raw: number): number {
  const pow = 10 ** Math.floor(Math.log10(Math.max(raw, 1)));
  const f = raw / pow;
  return (f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10) * pow;
}

/**
 * Net worth of every player over time, plain SVG. Lines are coloured by team;
 * the selected slot is drawn on top, thicker and white.
 */
export function NetWorthChart({
  i18n,
  series,
  teamOf,
  highlightSlot,
}: {
  i18n: I18n;
  series: SlotTimeline[];
  teamOf: (slot: number) => number;
  highlightSlot: number;
}) {
  const { t } = i18n;
  const points = series.flatMap((s) => s.points);
  if (points.length === 0) return <p className="muted">{t("table.noRows")}</p>;

  const maxT = Math.max(...points.map((p) => p.t), 60);
  const yStep = niceStep(Math.max(...points.map((p) => p.v), 1) / 4);
  const maxV = Math.ceil(Math.max(...points.map((p) => p.v), 1) / yStep) * yStep;
  const tStep = maxT > 2400 ? 600 : maxT > 900 ? 300 : 120;

  const x = (sec: number) => PAD.left + (sec / maxT) * (W - PAD.left - PAD.right);
  const y = (v: number) => H - PAD.bottom - (v / maxV) * (H - PAD.top - PAD.bottom);
  const path = (s: SlotTimeline) =>
    [{ t: 0, v: 0 }, ...s.points].map((p, i) => `${i ? "L" : "M"}${x(p.t).toFixed(1)},${y(p.v).toFixed(1)}`).join("");

  const yTicks = Array.from({ length: Math.round(maxV / yStep) + 1 }, (_, i) => i * yStep);
  const tTicks = Array.from({ length: Math.floor(maxT / tStep) + 1 }, (_, i) => i * tStep);
  // Selected line last so it is drawn above the others.
  const ordered = [...series].sort((a, b) => Number(a.slot === highlightSlot) - Number(b.slot === highlightSlot));

  return (
    <>
      <div className="legend">
        <span>
          <i style={{ background: "var(--amber)" }} />
          {i18n.team(0)}
        </span>
        <span>
          <i style={{ background: "var(--sapphire)" }} />
          {i18n.team(1)}
        </span>
        <span>
          <i style={{ background: "var(--text)", height: 3 }} />
          {t("match.you")}
        </span>
      </div>
      <svg className="chart" viewBox={`0 0 ${W} ${H}`} role="img" aria-label={t("match.chartLabel")}>
        {yTicks.map((v) => (
          <g key={`y${v}`}>
            <line className="grid" x1={PAD.left} x2={W - PAD.right} y1={y(v)} y2={y(v)} />
            <text x={PAD.left - 8} y={y(v) + 4} textAnchor="end">
              {v >= 1000 ? `${i18n.num(v / 1000)}k` : v}
            </text>
          </g>
        ))}
        {tTicks.map((sec) => (
          <text key={`t${sec}`} x={x(sec)} y={H - 6} textAnchor="middle">
            {duration(sec)}
          </text>
        ))}
        {ordered.map((s) => {
          const me = s.slot === highlightSlot;
          return (
            <path
              key={s.slot}
              d={path(s)}
              fill="none"
              stroke={me ? "var(--text)" : teamOf(s.slot) === 0 ? "var(--amber)" : "var(--sapphire)"}
              strokeWidth={me ? 3 : 1.25}
              strokeOpacity={me ? 1 : 0.55}
              strokeLinejoin="round"
            />
          );
        })}
      </svg>
    </>
  );
}
