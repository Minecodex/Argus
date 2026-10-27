import { readFileSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";

const enabled = process.env.ARGUS_PLAN9_B01_E2E === "1";
test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
type RunMap = {
  hosts: Record<string, { host_id: string; name: string }>;
  dashboards: Record<string, { id: string; name: string }>;
  alert_policies: Record<string, { id: string; name: string }>;
};
const readRun = () =>
  JSON.parse(
    readFileSync(process.env.ARGUS_PLAN9_RESOURCE_MAP!, "utf8"),
  ) as RunMap;

async function noHorizontalOverflow(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth + 1,
    ),
  ).toBe(true);
}

test.describe("Plan9 B01 deployed real monitoring", () => {
  test.skip(!enabled, "requires the live, normally enrolled B01 run");
  test.describe.configure({ timeout: 90_000 });

  test.beforeEach(async ({ context }) => {
    const session = JSON.parse(
      readFileSync(process.env.ARGUS_PLAN9_SESSION_PATH!, "utf8"),
    ) as {
      cookies: Array<{
        name: string;
        value: string;
        domain: string;
        path: string;
        secure: boolean;
      }>;
    };
    await context.addCookies(
      session.cookies.map((cookie) => ({
        ...cookie,
        httpOnly: true,
        sameSite: "Lax" as const,
      })),
    );
  });

  for (const locale of ["zh-CN", "en-US"] as const) {
    for (const theme of ["light", "dark"] as const) {
      test(`${locale} ${theme}: hosts, four dashboards and alerts`, async ({
        page,
      }, info) => {
        const run = readRun();
        await page.setViewportSize({ width: 1440, height: 900 });
        await page.addInitScript(
          ({ locale, theme }) => {
            localStorage.setItem("argus.locale", locale);
            localStorage.setItem("argus.theme", theme);
          },
          { locale, theme },
        );
        const errors: string[] = [];
        const queryFailures: Array<{ dashboard: string; panels: string[] }> =
          [];
        page.on("pageerror", (error) => errors.push(error.message));
        for (const [key, host] of Object.entries(run.hosts)) {
          await page.goto(`/hosts/${host.host_id}`);
          await expect(
            page.getByText(host.name, { exact: true }).first(),
          ).toBeVisible();
          await expect(page.locator("html")).toHaveAttribute(
            "data-theme",
            theme,
          );
          await page.getByRole("tab", { name: /组件|Components/i }).click();
          await expect(page.locator("tbody tr").first()).toBeVisible();
          await noHorizontalOverflow(page);
          await page.screenshot({
            path: info.outputPath(`${key}-${locale}-${theme}.png`),
            fullPage: true,
          });
        }
        for (const [key, dashboard] of Object.entries(run.dashboards)) {
          const response = page.waitForResponse(
            (r) =>
              r.url().includes(`/dashboards/${dashboard.id}/execute`) &&
              r.request().method() === "POST",
          );
          await page.goto(
            `/dashboards/${dashboard.id}?resource=${run.hosts["host-a"].host_id}`,
          );
          const actual = await response;
          expect(actual.status()).toBe(200);
          const data = (await actual.json()) as {
            panels: Array<{ status: string }>;
          };
          expect(data.panels.length).toBeGreaterThan(0);
          const incomplete = data.panels.filter(
            (panel) => panel.status !== "success",
          );
          if (incomplete.length)
            queryFailures.push({
              dashboard: key,
              panels: incomplete.map((panel) => panel.status),
            });
          await expect(page.locator("[data-panel-id]")).toHaveCount(
            data.panels.length,
          );
          await expect(
            page.getByRole("button", { name: /查询中|Querying/i }),
          ).toHaveCount(0);
          await expect(page.locator("html")).toHaveAttribute("lang", locale);
          await expect(
            page.getByRole("button", {
              name: /^(数据时间与完整性|Data time and completeness)$/,
            }),
          ).toHaveCount(0);
          await expect(
            page.locator(".argus-dashboard-panel-header small"),
          ).toHaveCount(0);
          await expect(page.locator(".argus-dashboard-grid")).not.toContainText(
            /otel_scope_name|otel_scope_version|resource_service_instance_id|QUERY_BUDGET_EXCEEDED/,
          );
          for (const chart of await page
            .locator(".argus-observation-chart")
            .all()) {
            const size = await chart.boundingBox();
            expect(size!.height).toBeGreaterThanOrEqual(140);
            expect(size!.width).toBeGreaterThan(300);
          }
          if (key === "host.overview" || key === "collector.overview") {
            const stat = page.locator(
              `[data-panel-id="${key === "host.overview" ? "memory_bytes" : "uptime"}"] .argus-observation-stat`,
            );
            const count = await stat.count();
            expect(count).toBeGreaterThan(0);
            if (count === 1) await expect(stat.locator("span")).toHaveCount(0);
            for (const card of await stat.all()) {
              await expect(card.locator("strong")).not.toHaveText("—");
              const centered = await card.evaluate((el) => {
                const box = el.getBoundingClientRect(),
                  value = el.querySelector("strong")!.getBoundingClientRect();
                // Multiple business series have a caption; a single value sits at the exact center.
                return (
                  Math.abs(box.x + box.width / 2 - value.x - value.width / 2) <
                    3 &&
                  (el.querySelector("span") !== null ||
                    Math.abs(
                      box.y + box.height / 2 - value.y - value.height / 2,
                    ) < 3)
                );
              });
              expect(centered).toBe(true);
            }
          }
          const explanation = page
            .locator(".argus-dashboard-panel-title .argus-info-trigger")
            .first();
          await explanation.click();
          await expect(
            page.getByRole("dialog", { name: /查询说明|Query information/ }),
          ).toBeVisible();
          await expect(
            page.getByRole("dialog", { name: /查询说明|Query information/ }),
          ).toContainText(/摄入完整性|Ingestion completeness/);
          await page.keyboard.press("Escape");
          await expect(
            page.getByRole("dialog", { name: /查询说明|Query information/ }),
          ).toHaveCount(0);
          await page
            .locator("[data-panel-id]")
            .first()
            .locator(".argus-dashboard-panel-actions button")
            .first()
            .click();
          const expanded = page
            .getByRole("dialog")
            .filter({ has: page.locator(".argus-dashboard-detail-content") });
          await expect(expanded).toBeVisible();
          await expect(
            expanded.locator(".argus-dialog__top .argus-info-trigger"),
          ).toBeVisible();
          await expanded.locator(".argus-info-trigger").click();
          await expect(
            page.getByRole("dialog", { name: /查询说明|Query information/ }),
          ).toContainText(/摄入完整性|Ingestion completeness/);
          await page.keyboard.press("Escape");
          await expect(
            page.getByRole("dialog", { name: /查询说明|Query information/ }),
          ).toHaveCount(0);
          await expanded
            .getByRole("button", { name: /^关闭$|^Close$/ })
            .click();
          await expect(expanded).toHaveCount(0);
          await noHorizontalOverflow(page);
          await page.screenshot({
            path: info.outputPath(`${key}-${locale}-${theme}.png`),
            fullPage: true,
          });
        }
        await page.goto(
          `/alerts?tab=active&policy_id=${run.alert_policies.service.id}`,
        );
        await expect(
          page
            .getByText(run.alert_policies.service.name, { exact: true })
            .first(),
        ).toBeVisible();
        await noHorizontalOverflow(page);
        await page.screenshot({
          path: info.outputPath(`alerts-${locale}-${theme}.png`),
          fullPage: true,
        });
        await page
          .getByText(run.alert_policies.service.name, { exact: true })
          .first()
          .click();
        const drawer = page.getByRole("dialog", {
          name: run.alert_policies.service.name,
        });
        await expect(drawer).toBeVisible();
        const chart = drawer.locator(".argus-chart svg");
        await expect(chart).toBeVisible();
        const geometry = await chart.evaluate((el) => {
          const svg = el as SVGSVGElement,
            line = svg.querySelector(".argus-chart__grid")!;
          return {
            fraction:
              (Number(line.getAttribute("x2")) -
                Number(line.getAttribute("x1"))) /
              svg.viewBox.baseVal.width,
            ticks: [...svg.querySelectorAll(".argus-chart__tick")]
              .filter((e) => e.getAttribute("text-anchor") === "end")
              .map((e) => e.textContent),
          };
        });
        expect(geometry.fraction).toBeGreaterThan(0.8);
        expect(geometry.ticks.length).toBeGreaterThanOrEqual(4);
        expect(
          geometry.ticks.every((t) => t && Number.isFinite(Number(t))),
        ).toBe(true);
        await expect(drawer.locator("pre.argus-alert-code-block")).toHaveCSS(
          "white-space",
          "pre",
        );
        await page.screenshot({
          path: info.outputPath(`alert-detail-${locale}-${theme}.png`),
          fullPage: true,
        });
        const timelineTab = drawer.getByRole("tab", {
          name: /^时间线$|^Timeline$/,
        });
        await timelineTab.click();
        await expect(timelineTab).toHaveAttribute("aria-selected", "true");
        await expect(
          drawer.locator(".argus-alert-timeline li").first(),
        ).toBeVisible();
        await expect(drawer.locator(".argus-alert-timeline pre")).toHaveCount(
          0,
        );
        await expect(drawer.locator(".argus-alert-timeline")).not.toContainText(
          /handling_version|authorization_version|policy_snapshot|resource_ids/,
        );
        await expect(
          drawer.locator(".argus-alert-timeline time").first(),
        ).toBeVisible();
        await page.screenshot({
          path: info.outputPath(`alert-timeline-${locale}-${theme}.png`),
          fullPage: true,
        });
        expect(errors).toEqual([]);
        expect(
          queryFailures,
          "B01 default dashboard data completeness",
        ).toEqual([]);
      });
    }
  }

  for (const width of [1280, 1920]) {
    test(`host dashboard fits desktop width ${width}`, async ({
      page,
    }, info) => {
      const run = readRun();
      await page.setViewportSize({ width, height: 900 });
      const response = page.waitForResponse(
        (r) =>
          r
            .url()
            .includes(
              `/dashboards/${run.dashboards["host.overview"].id}/execute`,
            ) && r.request().method() === "POST",
      );
      await page.goto(
        `/dashboards/${run.dashboards["host.overview"].id}?resource=${run.hosts["host-a"].host_id}`,
      );
      const actual = await response;
      expect(actual.status()).toBe(200);
      const execution = await actual.json();
      expect(
        execution.panels.every(
          (p: { status: string }) => p.status === "success",
        ),
      ).toBe(true);

      await expect(page.locator("[data-panel-id]").first()).toBeVisible();
      await expect(
        page.getByRole("button", { name: /查询中|Querying/i }),
      ).toHaveCount(0);
      await noHorizontalOverflow(page);
      await page.screenshot({
        path: info.outputPath(`desktop-${width}.png`),
        fullPage: true,
      });
    });
  }
});
