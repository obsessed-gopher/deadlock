import type { SlotTimeline } from "@/api/schema.gen.ts";
import { duration, num } from "@/lib/format.ts";

const W = 800;
const H = 280;
const PAD = { top: 12, right: 16, bottom: 26, left: 52 };

/** Rounds a value up to a readable axis step: 1, 2, 5 × 10^n. */
function niceStep(raw: number): number {
  const pow = 10 ** Math.floor(Math.log10(Math.max(raw, 1)));
  const f = raw / pow;
  return (f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10) * pow;
}

/**
 * Net worth of every player over time, plain SVG. Lines are coloured by team;
 * the highlighted slot is drawn on top and thicker.
 */
export function NetWorthChart({
  series,
  teamOf,
  highlightSlot,
}: {
  series: SlotTimeline[];
  teamOf: (slot: number) => number;
  highlightSlot: number;
}) {
  const points = series.flatMap((s) => s.points);
  if (points.length === 0) return <p className="muted">Нет данных.</p>;

  const maxT = Math.max(...points.map((p) => p.t), 60);
  const yStep = niceStep(Math.max(...points.map((p) => p.v), 1) / 5);
  const maxV = Math.ceil(Math.max(...points.map((p) => p.v), 1) / yStep) * yStep;
  const tStep = maxT > 2400 ? 600 : maxT > 900 ? 300 : 120;

  const x = (t: number) => PAD.left + (t / maxT) * (W - PAD.left - PAD.right);
  const y = (v: number) => H - PAD.bottom - (v / maxV) * (H - PAD.top - PAD.bottom);
  const path = (s: SlotTimeline) =>
    [{ t: 0, v: 0 }, ...s.points].map((p, i) => `${i ? "L" : "M"}${x(p.t).toFixed(1)},${y(p.v).toFixed(1)}`).join("");

  const yTicks = Array.from({ length: Math.round(maxV / yStep) + 1 }, (_, i) => i * yStep);
  const tTicks = Array.from({ length: Math.floor(maxT / tStep) + 1 }, (_, i) => i * tStep);
  // Highlighted line last so it is drawn above the others.
  const ordered = [...series].sort((a, b) => Number(a.slot === highlightSlot) - Number(b.slot === highlightSlot));

  return (
    <svg className="chart" viewBox={`0 0 ${W} ${H}`} role="img" aria-label="Души игроков по времени">
      {yTicks.map((v) => (
        <g key={`y${v}`}>
          <line className="grid" x1={PAD.left} x2={W - PAD.right} y1={y(v)} y2={y(v)} />
          <text x={PAD.left - 6} y={y(v) + 4} textAnchor="end">
            {v >= 1000 ? `${num(v / 1000)}k` : v}
          </text>
        </g>
      ))}
      {tTicks.map((t) => (
        <text key={`t${t}`} x={x(t)} y={H - 8} textAnchor="middle">
          {duration(t)}
        </text>
      ))}
      {ordered.map((s) => {
        const me = s.slot === highlightSlot;
        return (
          <path
            key={s.slot}
            d={path(s)}
            fill="none"
            stroke={teamOf(s.slot) === 0 ? "var(--amber)" : "var(--sapphire)"}
            strokeWidth={me ? 3 : 1.25}
            strokeOpacity={me ? 1 : 0.45}
            strokeLinejoin="round"
          />
        );
      })}
    </svg>
  );
}
