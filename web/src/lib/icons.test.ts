import assert from "node:assert/strict";
import { mkdtemp, mkdir, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";

import { GET } from "../app/icons/[kind]/[key]/route.ts";
import { iconUrl, initials, parseIconRequest, placeholderSvg } from "./icons.ts";

test("icon requests: only known kinds and game-key characters", () => {
  assert.deepEqual(parseIconRequest("heroes", "hero_haze"), { kind: "heroes", key: "hero_haze" });
  assert.deepEqual(parseIconRequest("abilities", "upgrade_x.png"), { kind: "abilities", key: "upgrade_x" });
  for (const [kind, key] of [
    ["items", "x"],
    ["heroes", "../secret"],
    ["heroes", "Hero"],
    ["heroes", ""],
    ["heroes", "a".repeat(81)],
  ]) {
    assert.equal(parseIconRequest(kind!, key!), null, `${kind}/${key}`);
  }
  assert.equal(iconUrl("heroes", "hero_haze"), "/icons/heroes/hero_haze");
});

test("initials drop the key prefix", () => {
  assert.equal(initials("hero_haze"), "H");
  assert.equal(initials("upgrade_rapid_rounds"), "RR");
  assert.equal(initials("ability_smoke_bomb_extra"), "SB");
  assert.equal(initials("hero_"), "?");
  assert.match(placeholderSvg("upgrade_rapid_rounds"), /^<svg[^>]*>.*>RR<\/text><\/svg>$/);
});

async function get(kind: string, key: string) {
  return GET(new Request(`http://x/icons/${kind}/${key}`), { params: Promise.resolve({ kind, key }) });
}

test("route serves the PNG, a placeholder when missing, 404 for bad requests", async () => {
  const dir = await mkdtemp(join(tmpdir(), "icons-"));
  await mkdir(join(dir, "heroes"));
  await writeFile(join(dir, "heroes", "hero_haze.png"), Buffer.from("fake png"));
  const prev = process.env.ICONS_DIR;
  process.env.ICONS_DIR = dir;
  try {
    const found = await get("heroes", "hero_haze");
    assert.equal(found.status, 200);
    assert.equal(found.headers.get("content-type"), "image/png");
    assert.equal(await found.text(), "fake png");

    const missing = await get("abilities", "upgrade_rapid_rounds");
    assert.equal(missing.status, 200);
    assert.equal(missing.headers.get("content-type"), "image/svg+xml");
    assert.match(await missing.text(), />RR</);

    assert.equal((await get("items", "x")).status, 404);
    assert.equal((await get("heroes", "..%2Fx")).status, 404);
  } finally {
    if (prev === undefined) delete process.env.ICONS_DIR;
    else process.env.ICONS_DIR = prev;
  }
});
