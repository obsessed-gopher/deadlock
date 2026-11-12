// Hero, item and ability icons. The PNGs are extracted from the local game
// install (go run ./tools/gamedata -icons data/icons) and are not in git, so
// every icon may be missing: the /icons route then answers with a generated
// placeholder instead of a 404, and pages never show broken images.

export const ICON_KINDS = ["heroes", "abilities"] as const;
export type IconKind = (typeof ICON_KINDS)[number];

export type IconTile = "weapon" | "vitality" | "spirit" | "ability";

/** Tile of an item or ability glyph: shop items by slot, the rest neutral. */
export function tileOf(slot: string | undefined, isItem: boolean): IconTile {
  return isItem && (slot === "weapon" || slot === "vitality" || slot === "spirit") ? slot : "ability";
}

export function iconUrl(kind: IconKind, key: string): string {
  return `/icons/${kind}/${key}`;
}

/** Validates route parameters; keys become file names, so only game-key characters. */
export function parseIconRequest(kind: string, key: string): { kind: IconKind; key: string } | null {
  const k = key.endsWith(".png") ? key.slice(0, -4) : key;
  if (!(ICON_KINDS as readonly string[]).includes(kind)) return null;
  if (!/^[a-z0-9_]{1,80}$/.test(k)) return null;
  return { kind: kind as IconKind, key: k };
}

const PREFIXES = ["hero_", "upgrade_", "ability_", "citadel_", "item_"];

/** "hero_haze" -> "H", "upgrade_rapid_rounds" -> "RR". */
export function initials(key: string): string {
  let name = key;
  for (const p of PREFIXES) if (name.startsWith(p)) name = name.slice(p.length);
  const words = name.split("_").filter((w) => /^[a-z]/.test(w));
  return (words.slice(0, 2).map((w) => w[0]!.toUpperCase()).join("") || "?").slice(0, 2);
}

/** 64x64 SVG tile with the key's initials, in the palette of the UI. */
export function placeholderSvg(key: string): string {
  const text = initials(key);
  return (
    '<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64" viewBox="0 0 64 64">' +
    '<rect width="64" height="64" rx="14" fill="#1f1f1f"/>' +
    `<text x="32" y="41" text-anchor="middle" font-family="system-ui,sans-serif" font-size="24" font-weight="600" fill="#a1a1a1">${text}</text>` +
    "</svg>"
  );
}
