import { readFileSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";

test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
type Map = {
  targets: Record<string, { name: string; target_id: string; engine: string }>;
  dashboards: Record<string, { id: string }>;
  alert_policies: Record<string, { id: string; name: string }>;
};
const run = () =>
  JSON.parse(
    readFileSync(process.env.ARGUS_PLAN9_RESOURCE_MAP!, "utf8"),
  ) as Map;
async function fits(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  ).toBe(true);
}

test.describe("Plan9 B02b deployed web services", () => {
  test.skip(
    process.env.ARGUS_PLAN9_B02B_E2E !== "1",
    "requires normally enrolled real B02b services",
  );
  test.describe.configure({ timeout: 120_000 });
  test.beforeEach(async ({ context }) => {
    const session = JSON.parse(
      readFileSync(process.env.ARGUS_PLAN9_SESSION_PATH!, "utf8"),
    );
    await context.addCookies(
      session.cookies.map((c: object) => ({
        ...c,
        httpOnly: true,
        sameSite: "Lax",
      })),
    );
  });
  for (const locale of ["zh-CN", "en-US"])
    for (const theme of ["light", "dark"]) {
      test(`${locale} ${theme}: ten native targets, dashboards and alert history`, async ({
        page,
      }, info) => {
        const map = run();
        await page.setViewportSize({ width: 1440, height: 900 });
        await page.addInitScript(
          ({ locale, theme }) => {
            localStorage.setItem("argus.locale", locale);
            localStorage.setItem("argus.theme", theme);
          },
          { locale, theme },
        );
        const errors: string[] = [];
        page.on("pageerror", (e) => errors.push(e.message));
        const inventory = await page.request.get(
          "/api/v1/monitoring/collections?limit=100",
        );
        expect(inventory.status()).toBe(200);
        const inventoryRows: Array<{ target: { id: string } }> = (
          await inventory.json()
        ).items;
        const retired = JSON.parse(
          readFileSync(process.env.ARGUS_PLAN9_RETIRED_MAP!, "utf8"),
        ) as { hosts: Record<string, { host_id: string }> };
        const retiredIds = [
          ...Object.values(retired.hosts).map((h) => h.host_id),
          ...Object.values(retired.targets ?? {}).map((t) => t.target_id),
        ];
        expect(
          inventoryRows.filter((c) => retiredIds.includes(c.target.id)),
        ).toEqual([]);
        await page.goto("/monitoring");
        await expect(page.locator("html")).toHaveAttribute("lang", locale);
        await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
        for (const target of Object.values(map.targets))
          await expect(
            page.getByText(target.name, { exact: true }),
          ).toBeVisible();
        await fits(page);
        await page.screenshot({
          path: info.outputPath(`monitoring-${locale}-${theme}.png`),
          fullPage: true,
        });
        for (const [key, target] of Object.entries(map.targets)) {
          const board = map.dashboards[target.engine];
          const response = page.waitForResponse(
            (r) =>
              r.url().includes(`/dashboards/${board.id}/execute`) &&
              r.request().method() === "POST",
          );
          // Exercise the real monitoring-row navigation, including its typed target scope.
          await page.goto("/monitoring");
          const row = page.getByRole("row").filter({ hasText: target.name });
          await row
            .getByRole("button", { name: /默认仪表盘|Default dashboard/i })
            .click();
          await expect(page).toHaveURL(new RegExp(`/dashboards/${board.id}`));
          expect(new URL(page.url()).searchParams.get("resource")).toBe(
            target.target_id,
          );
          expect(new URL(page.url()).searchParams.get("resource_type")).toBe(
            "monitoring_target",
          );
          const actual = await response;
          expect(actual.status()).toBe(200);
          const data = await actual.json();
          expect(data.panels.length).toBe(
            { http: 4, tcp: 2, tls: 2, dns: 2, icmp: 5 }[target.engine],
          );
          for (const panel of data.panels) {
            expect(panel.status).toBe("success");
            for (const result of panel.targets) {
              const series = Array.isArray(result.data)
                ? result.data
                : result.data.result;
              expect(series.length).toBeGreaterThan(0);
              for (const item of series)
                if (item.metric?.argus_resource_id)
                  expect(item.metric.argus_resource_id).toBe(target.target_id);
            }
          }
          await expect(page.locator("[data-panel-id]")).toHaveCount(
            data.panels.length,
          );
          await expect(
            page.getByRole("button", { name: /查询中|Querying/ }),
          ).toHaveCount(0);
          const headers = await page
            .locator(".argus-dashboard-panel-header")
            .allTextContents();
          expect(headers.join(" ")).not.toMatch(
            /topk\(|timestamp\(|dnscheck_|httpcheck_|ping_rtt/,
          );
          await expect(
            page.locator(".argus-dashboard-panel-header small"),
          ).toHaveCount(0);
          await page
            .locator(".argus-dashboard-panel-title .argus-info-trigger")
            .first()
            .click();
          await expect(
            page.getByRole("dialog", { name: /查询说明|Query information/ }),
          ).toBeVisible();
          await page.keyboard.press("Escape");
          await expect(
            page.getByRole("dialog", { name: /查询说明|Query information/ }),
          ).toHaveCount(0);
          await fits(page);
          await page.screenshot({
            path: info.outputPath(`${key}-${locale}-${theme}.png`),
            fullPage: true,
          });
        }
        const policy = map.alert_policies.http;
        await page.goto(`/alerts?tab=active&policy_id=${policy.id}`);
        await page.getByText(policy.name, { exact: true }).first().click();
        const detail = page.getByRole("dialog", { name: policy.name });
        await expect(detail).toContainText(map.targets["target-a-http"].name);
        await expect(detail.locator(".argus-chart svg")).toBeVisible();
        await expect(detail.locator("pre.argus-alert-code-block")).toHaveCSS(
          "white-space",
          "pre",
        );
        await page.screenshot({
          path: info.outputPath(`alert-${locale}-${theme}.png`),
          fullPage: true,
        });
        await detail.getByRole("tab", { name: /^时间线$|^Timeline$/ }).click();
        await expect(
          detail.locator(".argus-alert-timeline li").first(),
        ).toBeVisible();
        await expect(detail.locator(".argus-alert-timeline pre")).toHaveCount(
          0,
        );
        await page.screenshot({
          path: info.outputPath(`timeline-${locale}-${theme}.png`),
          fullPage: true,
        });
        expect(errors).toEqual([]);
      });
    }
  for (const width of [1280, 1920])
    test(`HTTP failure dashboard fits ${width}`, async ({ page }, info) => {
      const map = run(),
        board = map.dashboards.http,
        target = map.targets["target-a-http"];
      await page.setViewportSize({ width, height: 900 });
      const response = page.waitForResponse(
        (r) =>
          r.url().includes(`/dashboards/${board.id}/execute`) &&
          r.request().method() === "POST",
      );
      await page.goto(
        `/dashboards/${board.id}?resource=${target.target_id}&resource_type=monitoring_target`,
      );
      const actual = await response;
      expect(actual.status()).toBe(200);
      expect(
        (await actual.json()).panels.every(
          (p: { status: string }) => p.status === "success",
        ),
      ).toBe(true);
      await expect(page.locator("[data-panel-id]")).toHaveCount(4);
      await fits(page);
      await page.screenshot({
        path: info.outputPath(`http-${width}.png`),
        fullPage: true,
      });
    });
});
