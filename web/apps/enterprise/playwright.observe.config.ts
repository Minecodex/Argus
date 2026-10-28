import { defineConfig, devices } from "@playwright/test";
for (const key of [
  "HTTP_PROXY",
  "HTTPS_PROXY",
  "ALL_PROXY",
  "http_proxy",
  "https_proxy",
  "all_proxy",
])
  delete process.env[key];
process.env.NO_PROXY = "127.0.0.1,localhost";
export default defineConfig({
  testDir: "./e2e",
  testMatch: "observe-real.spec.ts",
  retries: 0,
  reporter: "list",
  timeout: 120000,
  outputDir: "../../../.cache/observe-real-results",
  use: {
    ...devices["Desktop Chrome"],
    baseURL: "http://127.0.0.1:4278",
    trace: "off",
    actionTimeout: 20000,
  },
  webServer: {
    command: "node ../../../scripts/run-vite.mjs enterprise 4278 real",
    url: "http://127.0.0.1:4278",
    reuseExistingServer: false,
    env: {
      VITE_API_BASE_URL: process.env.VITE_API_BASE_URL!,
      VITE_TEMPLATE_ORIGIN: "http://127.0.0.1:4278",
    },
  },
});
