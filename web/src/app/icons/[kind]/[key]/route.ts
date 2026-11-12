// GET /icons/<heroes|abilities>/<key>: the extracted PNG from ICONS_DIR, or a
// generated placeholder when it is missing (never a 404 for a valid key).
// Relative imports only: node --test imports this file directly.
import { readFile } from "node:fs/promises";
import { join } from "node:path";

import { parseIconRequest, placeholderSvg } from "../../../../lib/icons.ts";

function iconsDir(): string {
  // Docker sets ICONS_DIR=/icons; locally web/ runs next to the repo's data/.
  return process.env.ICONS_DIR || join(process.cwd(), "..", "data", "icons");
}

export async function GET(_req: Request, { params }: { params: Promise<{ kind: string; key: string }> }) {
  const { kind, key } = await params;
  const icon = parseIconRequest(kind, key);
  if (!icon) return new Response("not found", { status: 404 });
  try {
    const png = await readFile(join(iconsDir(), icon.kind, `${icon.key}.png`));
    return new Response(new Uint8Array(png), {
      headers: { "content-type": "image/png", "cache-control": "public, max-age=86400" },
    });
  } catch {
    // Short cache: icons may be extracted later without a restart.
    return new Response(placeholderSvg(icon.key), {
      headers: { "content-type": "image/svg+xml", "cache-control": "public, max-age=300" },
    });
  }
}
