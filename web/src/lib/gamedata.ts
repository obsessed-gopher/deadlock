import type { Ability, GameData, Hero } from "../api/schema.gen.ts";
import type { I18n } from "../i18n/core.ts";

/** Id-indexed view of GetGameData with names in the chosen language. */
export interface Lookup {
  hero(id: number): Hero | undefined;
  heroName(id: number): string;
  ability(id: number): Ability | undefined;
  /** Display name of an ability or item. */
  abilityName(a: Ability): string;
  /** Playable heroes sorted by display name, for filters. */
  playableHeroes: Hero[];
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
    playableHeroes: gd.heroes
      .filter((h) => h.playable)
      .sort((a, b) => i18n.name(a).localeCompare(i18n.name(b), i18n.locale)),
  };
}
