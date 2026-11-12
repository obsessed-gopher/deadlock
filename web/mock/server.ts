// Mock REST API for local development and e2e tests: serves mock/fixtures
// with the same routes and JSON as the Go gateway. No Postgres, no network.
//
//   node mock/server.ts            # port from MOCK_PORT, default 8090
import { createServer } from "node:http";
import { join } from "node:path";

import { createMockApi, loadFixtures } from "./api.ts";

const port = Number(process.env.MOCK_PORT ?? 8090);
const fixtures = join(import.meta.dirname, "fixtures");
const api = createMockApi(loadFixtures(fixtures));

const server = createServer((req, res) => {
  const { status, body } = api.handle(req.method ?? "GET", req.url ?? "/");
  // Drain the request body (sync POSTs send "{}").
  req.resume();
  res.writeHead(status, { "content-type": "application/json" });
  res.end(JSON.stringify(body));
});

server.listen(port, () => {
  console.log(`mock api: http://localhost:${port} (fixtures: ${fixtures})`);
});

for (const sig of ["SIGINT", "SIGTERM"] as const) {
  process.on(sig, () => server.close(() => process.exit(0)));
}
