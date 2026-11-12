import type { Ability, GameData, Hero } from "../api/schema.gen.ts";

/** Id-indexed view of GetGameData with display-name fallbacks. */
export interface Lookup {
  hero(id: number): Hero | undefined;
  heroName(id: number): string;
  ability(id: number): Ability | undefined;
  /** Playable heroes sorted by display name, for filters. */
  playableHeroes: Hero[];
}

export function displayName(x: { name: string; nameRu: string; key: string }): string {
  return x.nameRu || x.name || x.key;
}

export function makeLookup(gd: GameData): Lookup {
  const heroes = new Map(gd.heroes.map((h) => [h.id, h]));
  const abilities = new Map(gd.abilities.map((a) => [a.id, a]));
  return {
    hero: (id) => heroes.get(id),
    heroName: (id) => {
      const h = heroes.get(id);
      return h ? displayName(h) : `Герой #${id}`;
    },
    ability: (id) => abilities.get(id),
    playableHeroes: gd.heroes
      .filter((h) => h.playable)
      .sort((a, b) => displayName(a).localeCompare(displayName(b), "ru")),
  };
}
