// Snapshots a running API into mock/fixtures, so the mock server and e2e tests
// work on real-looking data without Postgres or network access.
//
//   node mock/capture.ts [api-url] [account-id]
//
// Defaults: http://localhost:8080, 1517505834. Takes the newest 120 history
// rows, their match facts, the peers aggregate (whole history) and full
// breakdowns (all players) of the 3 newest matches with metadata. Run it only
// by hand: it talks to a real server.
import { mkdir, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";

import type {
  GameData,
  ListPlayerMatchFactsResponse,
  ListPlayerMatchesResponse,
  ListPlayerPeersResponse,
  Match,
  PlayerMatch,
} from "../src/api/schema.gen.ts";

const api = (process.argv[2] ?? "http://localhost:8080").replace(/\/+$/, "");
const account = process.argv[3] ?? "1517505834";
const dir = join(import.meta.dirname, "fixtures");
const ROWS = 120;
const FULL_MATCHES = 3;

async function get<T>(path: string): Promise<T> {
  const res = await fetch(api + path);
  if (!res.ok) throw new Error(`${path}: ${res.status} ${await res.text()}`);
  return (await res.json()) as T;
}

async function save(name: string, data: unknown): Promise<void> {
  await writeFile(join(dir, name), JSON.stringify(data) + "\n");
}

await rm(dir, { recursive: true, force: true });
await mkdir(join(dir, "player-matches"), { recursive: true });

await save("gamedata.json", await get<GameData>("/v1/gamedata"));

const rows = (await get<ListPlayerMatchesResponse>(`/v1/players/${account}/matches?pageSize=${ROWS}`)).matches;
await save("matches.json", { accountId: Number(account), matches: rows });

const ids = new Set(rows.map((r) => r.matchId));
const facts = await get<ListPlayerMatchFactsResponse>(`/v1/players/${account}/facts`);
await save("facts.json", { ...facts, matches: facts.matches.filter((f) => ids.has(f.matchId)) });

await save("peers.json", await get<ListPlayerPeersResponse>(`/v1/players/${account}/peers`));

const full = rows.filter((r) => r.metaStatus === "META_STATUS_OK").slice(0, FULL_MATCHES);
for (const r of full) {
  const match = await get<Match>(`/v1/matches/${r.matchId}`);
  for (const p of match.players) {
    const pm = await get<PlayerMatch>(`/v1/matches/${r.matchId}/players/${p.accountId}`);
    await save(`player-matches/${r.matchId}-${p.accountId}.json`, pm);
  }
}
console.log(`fixtures: ${rows.length} rows, ${full.length} full matches -> ${dir}`);
