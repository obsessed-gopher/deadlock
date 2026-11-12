// Local development with hot reload on mock data: starts mock/server.ts and
// `next dev` pointed at it. Ctrl+C stops both.
//
//   npm run dev:mock      # http://localhost:3002, mock API on :8092
//
// Ports differ from the production (3000/8080) and dev compose (3001/8090)
// stacks, so all three can run at once.
import { type ChildProcess, spawn } from "node:child_process";
import { join } from "node:path";

const root = join(import.meta.dirname, "..");
const mockPort = process.env.MOCK_PORT ?? "8092";
const webPort = process.env.WEB_PORT ?? "3002";
const env: NodeJS.ProcessEnv = {
  SYNC_ACCOUNTS: "1517505834", // the account in mock/fixtures
  ...process.env,
  MOCK_PORT: mockPort,
  API_URL: `http://localhost:${mockPort}`,
};

const children: ChildProcess[] = [
  spawn(process.execPath, ["mock/server.ts"], { cwd: root, env, stdio: "inherit" }),
  // shell: npx is a .cmd script on Windows.
  spawn("npx", ["next", "dev", "-p", webPort], { cwd: root, env, stdio: "inherit", shell: true }),
];

let stopping = false;
function stop(code: number) {
  if (stopping) return;
  stopping = true;
  for (const c of children) c.kill();
  process.exit(code);
}
for (const c of children) c.on("exit", (code) => stop(code ?? 0));
process.on("SIGINT", () => stop(0));
process.on("SIGTERM", () => stop(0));
