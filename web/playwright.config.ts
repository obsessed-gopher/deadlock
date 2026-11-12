import { defineConfig, devices } from "@playwright/test";

// E2E: the production build of the app against the mock API (mock/server.ts),
// so tests are deterministic and need neither Postgres nor network.
// Run with "npm run e2e" (builds first) or "go run ./tools/dev e2e".
const MOCK_PORT = 8091;
const WEB_PORT = 3200;
const CI = !!process.env.CI;

export default defineConfig({
  testDir: "e2e",
  fullyParallel: true,
  forbidOnly: CI,
  retries: CI ? 1 : 0,
  reporter: CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL: `http://localhost:${WEB_PORT}`,
    locale: "ru-RU",
    timezoneId: "UTC",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    { name: "desktop", use: { ...devices["Desktop Chrome"] } },
    { name: "mobile", use: { ...devices["Pixel 7"] } },
  ],
  webServer: [
    {
      command: "node mock/server.ts",
      url: `http://localhost:${MOCK_PORT}/healthz`,
      env: { MOCK_PORT: String(MOCK_PORT) },
      reuseExistingServer: !CI,
    },
    {
      // The same standalone server the Docker image runs (npm run e2e builds it).
      command: "node .next/standalone/server.js",
      url: `http://localhost:${WEB_PORT}`,
      // Dates are rendered on the server: pin the zone so assertions are stable.
      env: {
        PORT: String(WEB_PORT),
        HOSTNAME: "localhost",
        API_URL: `http://localhost:${MOCK_PORT}`,
        SYNC_ACCOUNTS: "1517505834",
        TZ: "UTC",
        // No game icons in CI: tests run against the placeholders on every machine.
        ICONS_DIR: "e2e/no-icons",
      },
      reuseExistingServer: !CI,
      timeout: 60_000,
    },
  ],
});
