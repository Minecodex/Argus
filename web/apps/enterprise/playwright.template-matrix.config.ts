import { defineConfig } from "@playwright/test";
import base from "./playwright.config";

// The full desktop matrix reuses prepared compiled mock portal previews and
// the compiled Template Runtime. These two fault-injection cases specifically
// intercept a Vite source module, so they run against a separate dev origin.
// Every original test is assigned once; no assertion or skip is removed.
const devPort = process.env.ARGUS_E2E_SOURCE_FIXTURE_PORT ?? "4175";
const devOrigin = `http://127.0.0.1:${devPort}`;
const inherited = Array.isArray(base.webServer)
  ? base.webServer
  : base.webServer
    ? [base.webServer]
    : [];
const desktop = base.projects![0]!;
export default defineConfig({
  ...base,
  webServer: [
    ...inherited,
    {
      command: `node ../../../scripts/run-vite.mjs enterprise ${devPort} mock`,
      url: devOrigin,
      env: {
        VITE_TEMPLATE_ORIGIN:
          process.env.ARGUS_E2E_TEMPLATE_ORIGIN ?? "http://127.0.0.1:4186",
      },
      reuseExistingServer: !process.env.CI,
    },
  ],
  projects: [
    { ...desktop, testIgnore: "**/host-callback-preflight.spec.ts", grepInvert: /redesign shared controls/ },
    {
      ...desktop,
      name: "chromium-source-fixtures",
      testMatch: "**/host-callback-preflight.spec.ts",
      use: { ...desktop.use, baseURL: devOrigin },
    },
    {
      ...desktop,
      name: "chromium-development-gallery",
      testMatch: "**/frontend-redesign.spec.ts",
      grep: /redesign shared controls/,
      use: { ...desktop.use, baseURL: devOrigin },
    },
  ],
});
