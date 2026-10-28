import { defineConfig, devices } from "@playwright/test";
for (const key of [
  "http_proxy",
  "https_proxy",
  "all_proxy",
  "HTTP_PROXY",
  "HTTPS_PROXY",
  "ALL_PROXY",
])
  delete process.env[key];
process.env.NO_PROXY = "127.0.0.1,localhost";
export default defineConfig({
  testDir: "./e2e",
  testMatch: "model-governance-real.spec.ts",
  retries: 0,
  reporter: "list",
  timeout: 90_000,
  outputDir:
    process.env.ARGUS_MODEL_GOVERNANCE_BROWSER_OUTPUT ??
    "../../../build/model-governance-e2e/browser",
  use: {
    ...devices["Desktop Chrome"],
    baseURL: "http://127.0.0.1:4283",
    trace: "off",
  },
  webServer: {
    command: "node ../../../scripts/run-vite.mjs enterprise 4283 real",
    url: "http://127.0.0.1:4283",
    reuseExistingServer: false,
    env: {
      VITE_API_BASE_URL: process.env.VITE_API_BASE_URL!,
      VITE_TEMPLATE_ORIGIN: "http://127.0.0.1:4286",
    },
  },
});
