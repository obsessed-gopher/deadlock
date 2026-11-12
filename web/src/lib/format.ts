// Display helpers. Pure functions, unit-tested with node --test.

/** Game time or match length: 75 -> "1:15", 3725 -> "1:02:05". */
export function duration(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = String(s % 60).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${sec}` : `${m}:${sec}`;
}

const dateFmt = new Intl.DateTimeFormat("ru-RU", {
  day: "2-digit",
  month: "2-digit",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

/** Unix seconds (int64 comes as a decimal string) -> "12.11.2026, 18:43". 0 -> "—". */
export function dateTime(unix: string | number): string {
  const n = Number(unix);
  return n > 0 ? dateFmt.format(new Date(n * 1000)) : "—";
}

const numFmt = new Intl.NumberFormat("ru-RU");

/** 29130 -> "29 130" (ru-RU groups with a no-break space). */
export function num(n: number): string {
  return numFmt.format(n);
}

/** Percentage with one decimal; "—" when there is nothing to divide by. */
export function pct(part: number, whole: number): string {
  return whole > 0 ? `${((part / whole) * 100).toFixed(1)}%` : "—";
}

export function kda(k: number, d: number, a: number): string {
  return `${k} / ${d} / ${a}`;
}

/** (kills + assists) / deaths, deaths counted as at least 1. */
export function kdaRatio(k: number, d: number, a: number): string {
  return ((k + a) / Math.max(1, d)).toFixed(2);
}

// ECitadelLobbyTeam: Team0 = Amber (Hidden King), Team1 = Sapphire (Archmother).
export function teamName(team: number): string {
  return team === 0 ? "Янтарь" : team === 1 ? "Сапфир" : `Команда ${team}`;
}

// ECitadelGameMode in proto/deadlock/citadel_gcmessages_common.proto.
const gameModes: Record<number, string> = {
  1: "Обычный",
  2: "1v1",
  3: "Песочница",
  4: "Street Brawl",
};

export function gameModeName(mode: number): string {
  return gameModes[mode] ?? `Режим ${mode}`;
}

// ECitadelMatchMode in proto/deadlock/citadel_gcmessages_common.proto.
const matchModes: Record<number, string> = {
  1: "Без рейтинга",
  2: "Лобби",
  3: "Против ботов",
  4: "Рейтинговый",
  6: "Обучение",
  7: "Hero Labs",
  8: "Калибровка",
};

export function matchModeName(mode: number): string {
  return matchModes[mode] ?? `Тип ${mode}`;
}
