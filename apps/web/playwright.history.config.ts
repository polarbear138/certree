import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "tests",
  testMatch: ["history.spec.ts", "tree-operations.spec.ts"],
  workers: 1,
  use: {
    baseURL: "http://127.0.0.1:8088",
    headless: true,
    viewport: { width: 1440, height: 1000 },
  },
  webServer: {
    command: "npm run dev -- --port 8088 --strictPort",
    url: "http://127.0.0.1:8088",
    reuseExistingServer: false,
  },
  reporter: "list",
});
