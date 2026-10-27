import { test, expect } from "@playwright/test";
import fs from "node:fs/promises";
import { assertNoSeriousAxe } from "./helpers/portal-layout";

const enabled = process.env.ARGUS_PLAN8_WORKSPACE_REAL === "1";
test.skip(!enabled, "requires the owned, deployed Plan8 environment");
test.use({ viewport: { width: 1440, height: 1000 }, trace: "off" });

for (const width of [1280, 1440, 1920]) {
  for (const english of [false, true]) {
    for (const theme of ["light", "dark"]) {
      test(`Plan8 deployed workspace ${english ? "en" : "zh"} ${theme} ${width}`, async ({
        page,
        context,
      }, info) => {
        test.setTimeout(120_000);
        await page.setViewportSize({ width, height: 1000 });
        const origin = process.env.ARGUS_E2E_ENTERPRISE_ORIGIN!;
        await context.addCookies(
          JSON.parse(process.env.ARGUS_PLAN8_WORKSPACE_COOKIES!),
        );
        await page.addInitScript(
          ({ english, theme }) => {
            localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
            localStorage.setItem("argus.theme", theme);
          },
          { english, theme },
        );
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        const routes = [
          { path: "/applications", api: "/applications" },
          { path: "/monitoring", api: "/monitoring/" },
          { path: "/hosts", api: "/hosts" },
          { path: "/kubernetes", api: "/kubernetes-clusters" },
          { path: "/dashboards", api: "/dashboards" },
          { path: "/alerts", api: "/alerts/" },
        ];
        const observations: Array<{
          path: string;
          api_path: string;
          status: number;
        }> = [];
        for (const route of routes) {
          const actual = page.waitForResponse((response) => {
            const url = new URL(response.url());
            return (
              url.origin === new URL(origin).origin &&
              url.pathname.includes("/api/") &&
              url.pathname.includes(route.api) &&
              response.request().method() === "GET" &&
              response.status() === 200 &&
              (response.headers()["content-type"] ?? "").includes(
                "application/json",
              )
            );
          });
          await page.goto(origin + route.path);
          const response = await actual;
          await response.finished();
          await expect
            .poll(() => new URL(page.url()).pathname)
            .toMatch(new RegExp(`^${route.path}(?:/|$)`));
          await expect(
            page.getByRole("heading", { level: 1 }).first(),
          ).toBeVisible();
          expect(
            await page.evaluate(
              () => document.documentElement.scrollWidth <= window.innerWidth,
            ),
          ).toBe(true);
          await assertNoSeriousAxe(page);
          await page.screenshot({
            path: info.outputPath(`${route.path.slice(1)}.png`),
            fullPage: true,
          });
          observations.push({
            path: route.path,
            api_path: new URL(response.url()).pathname,
            status: response.status(),
          });
        }
        expect(errors).toEqual([]);
        await fs.writeFile(
          info.outputPath("workspace-evidence.json"),
          JSON.stringify({
            passed: true,
            scope:
              "actual_deployed_read_only_directories_existing_authorized_session",
            locale: english ? "en" : "zh",
            theme,
            desktop: { width, height: 1000 },
            accessibility: { existing_serious_critical_gate_passed: true },
            observations,
            writes: false,
          }),
        );
      });
    }
  }
}
