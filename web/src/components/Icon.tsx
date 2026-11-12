// Hero portrait or item/ability glyph. Glyphs are white on transparent in the
// game files and get a tile in the colour of their slot, as in the game.
// The image is decorative: the name is always next to it as text.

import type { IconTile } from "@/lib/icons.ts";

export function Icon({ src, tile, size = 24 }: { src: string; tile?: IconTile; size?: number }) {
  if (!tile) {
    return <img className="icon-portrait" src={src} alt="" width={size} height={size} loading="lazy" decoding="async" />;
  }
  const inner = Math.round(size * 0.78);
  return (
    <span className={`icon-tile tile-${tile}`} style={{ width: size, height: size }} aria-hidden="true">
      <img src={src} alt="" width={inner} height={inner} loading="lazy" decoding="async" />
    </span>
  );
}
