import type { Ability, GameData, Hero } from "../api/schema.gen.ts";
import type { I18n } from "../i18n/core.ts";
import { iconUrl } from "./icons.ts";

/** Id-indexed view of GetGameData with names in the chosen language. */
export interface Lookup {
  hero(id: number): Hero | undefined;
  heroName(id: number): string;
  ability(id: number): Ability | undefined;
  /** Display name of an ability or item. */
  abilityName(a: Ability): string;
  /** Playable heroes sorted by display name, for filters. */
  playableHeroes: Hero[];
  /** Icon URL of a known hero; undefined for ids the snapshot lacks. */
  heroIcon(id: number): string | undefined;
  abilityIcon(id: number): string | undefined;
}

export function makeLookup(gd: GameData, i18n: Pick<I18n, "name" | "t" | "locale">): Lookup {
  const heroes = new Map(gd.heroes.map((h) => [h.id, h]));
  const abilities = new Map(gd.abilities.map((a) => [a.id, a]));
  const heroName = (id: number) => {
    const h = heroes.get(id);
    return h ? i18n.name(h) : i18n.t("hero.unknown", { id });
  };
  return {
    hero: (id) => heroes.get(id),
    heroName,
    ability: (id) => abilities.get(id),
    abilityName: (a) => i18n.name(a),
    heroIcon: (id) => {
      const h = heroes.get(id);
      return h ? iconUrl("heroes", h.key) : undefined;
    },
    abilityIcon: (id) => {
      const a = abilities.get(id);
      return a ? iconUrl("abilities", a.key) : undefined;
    },
    playableHeroes: gd.heroes
      .filter((h) => h.playable)
      .sort((a, b) => i18n.name(a).localeCompare(i18n.name(b), i18n.locale)),
  };
}
