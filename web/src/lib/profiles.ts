// Steam names and avatars by account id. Optional everywhere: a missing
// profile (not fetched yet, private, API down) falls back to "Player <id>".
import type { Profile } from "../api/schema.gen.ts";
import type { I18n } from "../i18n/core.ts";

export type Profiles = ReadonlyMap<string, Profile>;

export const NO_PROFILES: Profiles = new Map();

export function profileMap(list: Profile[]): Profiles {
  return new Map(list.map((p) => [String(p.accountId), p]));
}

/** Steam name, or the localized "Player <id>". */
export function playerName(i18n: I18n, id: string | number, profiles: Profiles): string {
  const name = profiles.get(String(id))?.personaName.trim();
  return name || i18n.t("player.title", { id: String(id) });
}

/** Avatar URL of the given size, "" when unknown. */
export function avatarOf(id: string | number, profiles: Profiles, size: "small" | "medium" | "full" = "medium"): string {
  const p = profiles.get(String(id));
  if (!p) return "";
  return (size === "full" ? p.avatarFull : size === "small" ? p.avatar : p.avatarMedium) || p.avatar;
}

/** Unique ids in the order given, at most `max` (the API limit). */
export function uniqueIds(ids: Iterable<string | number>, max = 1000): string[] {
  return [...new Set([...ids].map(String))].slice(0, max);
}
