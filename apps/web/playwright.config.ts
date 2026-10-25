import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "tests",
  globalTeardown: "../../tests/e2e-teardown.mjs",
  timeout: 60000,
  workers: 1,
  use: {
    baseURL: "http://127.0.0.1:8087",
    headless: true,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    viewport: { width: 1440, height: 1000 },
  },
  webServer: {
    command: "node ../../tests/e2e-server.mjs",
    url: "http://127.0.0.1:8087/api/system/health",
    reuseExistingServer: false,
    timeout: 30000,
  },
  reporter: "list",
});
