// Locale-independent display helpers. Anything that depends on the language
// (numbers, dates, enum names) lives in i18n/core.ts.

/** Game time or match length: 75 -> "1:15", 3725 -> "1:02:05". */
export function duration(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = String(s % 60).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${sec}` : `${m}:${sec}`;
}

export function kda(k: number, d: number, a: number): string {
  return `${k} / ${d} / ${a}`;
}

/** (kills + assists) / deaths, deaths counted as at least 1. */
export function kdaRatio(k: number, d: number, a: number): number {
  return (k + a) / Math.max(1, d);
}
