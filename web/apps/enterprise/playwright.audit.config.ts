import { defineConfig, devices } from "@playwright/test";
import { readFileSync } from "node:fs";
const built = process.env.ARGUS_AUDIT_BUILT_UI === "1";
const api = built ? (JSON.parse(readFileSync(process.env.ARGUS_AUDIT_BROWSER_FIXTURE!, "utf8")) as { api: string }).api : "";
for (const name of [
  "HTTP_PROXY",
  "HTTPS_PROXY",
  "ALL_PROXY",
  "http_proxy",
  "https_proxy",
  "all_proxy",
])
  delete process.env[name];
process.env.NO_PROXY = "127.0.0.1,localhost";
export default defineConfig({
  testDir: "./e2e",
  testMatch: "audit-operations-real.spec.ts",
  retries: 0,
  reporter: "list",
  outputDir: "../../../.cache/audit-real-browser-results",
  timeout: 60000,
  use: {
    ...devices["Desktop Chrome"],
    baseURL: built ? new URL(api).origin : "http://127.0.0.1:4273",
    trace: "off",
  },
  webServer: built ? undefined : {
    command: "node ../../../scripts/run-vite.mjs enterprise 4273 real",
    url: "http://127.0.0.1:4273",
    reuseExistingServer: false,
    env: {
      VITE_API_BASE_URL: process.env.VITE_API_BASE_URL!,
      VITE_TEMPLATE_ORIGIN: "http://127.0.0.1:4276",
    },
  },
});
