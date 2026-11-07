import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";

test.use({ trace: "off", video: "off", reducedMotion: "reduce" });

type ResourceMap = {
  application: { id: string; name: string };
  hosts: { collector: { host_id: string } };
  dashboards: Record<"application" | "host", { id: string }>;
  log_rules: Record<string, { id: string; name: string }>;
  alert_policies: { service: { id: string; name: string } };
};

test.describe("Plan9 B08 native Linux logs", () => {
  test.skip(
    process.env.ARGUS_PLAN9_B08_E2E !== "1",
    "Requires the live B08 run",
  );
  test.describe.configure({ timeout: 90_000 });

  for (const locale of ["zh-CN", "en-US"])
    for (const theme of ["light", "dark"])
      test(locale + " " + theme, async ({ context, page }, info) => {
        const text = (zh: string, en: string) => (locale === "zh-CN" ? zh : en);
        const session = JSON.parse(
          readFileSync(process.env.ARGUS_PLAN9_SESSION_PATH!, "utf8"),
        );
        const map: ResourceMap = JSON.parse(
          readFileSync(process.env.ARGUS_PLAN9_RESOURCE_MAP!, "utf8"),
        );
        await context.addCookies(
          session.cookies.map((cookie: object) => ({
            ...cookie,
            httpOnly: true,
            sameSite: "Lax",
          })),
        );
        await page.setViewportSize({
          width: theme === "light" ? 1280 : 1920,
          height: 1050,
        });
        await page.addInitScript(
          ({ locale, theme }) => {
            localStorage.setItem("argus.locale", locale);
            localStorage.setItem("argus.theme", theme);
          },
          { locale, theme },
        );

        await page.goto("/monitoring");
        for (const rule of Object.values(map.log_rules)) {
          await expect(
            page.getByText(rule.name, { exact: true }),
          ).toBeVisible();
        }

        for (const kind of ["application", "host"] as const) {
          const resource =
            kind === "application"
              ? map.application.id
              : map.hosts.collector.host_id;
          const board = map.dashboards[kind].id;
          const executed = page.waitForResponse(
            (response) =>
              response.url().includes("/dashboards/" + board + "/execute") &&
              response.request().method() === "POST",
          );
          await page.goto(
            "/dashboards/" +
              board +
              "?resource=" +
              resource +
              "&resource_type=" +
              kind,
          );
          const response = await executed;
          expect(response.status()).toBe(200);
          const result = await response.json();
          expect(
            result.panels.map((p: { status: string }) => p.status),
          ).toEqual(kind === "host" ? ["success", "success"] : ["success"]);

          const panel = page.locator('[data-panel-id="logs"]');
          await panel
            .getByRole("textbox", {
              name: text("搜索当前结果", "Search current results"),
            })
            .fill(
              kind === "application"
                ? "caused by controlled dependency refusal"
                : "-journal",
            );
          const row = panel.getByRole("article").first();
          await expect(row).toContainText(
            kind === "application"
              ? "at orders.validate_request"
              : "plan9-b08.service",
          );
          await row
            .getByText(text("完整记录与字段", "Full record and fields"), {
              exact: true,
            })
            .click();
          await expect(row).toContainText(
            kind === "application" ? "filelog" : "journald",
          );
          expect(
            await page.evaluate(
              () => document.documentElement.scrollWidth <= innerWidth + 1,
            ),
          ).toBe(true);
          await page.screenshot({
            path: info.outputPath(kind + "-logs.png"),
            animations: "disabled",
          });
        }

        if (locale === "zh-CN" && theme === "light") {
          const host = map.hosts.collector.host_id;
          await page.goto(
            "/observe?resource_type=host&resource_id=" + host + "&signal=logs",
          );
          await expect(
            page.getByRole("button", { name: "采集来源", exact: true }),
          ).toHaveText("journald");
          await page
            .getByRole("button", { name: "执行全部", exact: true })
            .click();
          await expect(
            page.getByRole("region", { name: "日志结果" }),
          ).toContainText("plan9-b08.service");
          await page.screenshot({
            path: info.outputPath("host-log-search.png"),
            animations: "disabled",
          });
        }

        const alert = map.alert_policies.service;
        await page.goto("/alerts?tab=history&policy_id=" + alert.id);
        await page.getByText(alert.name, { exact: true }).first().click();
        const detail = page.getByRole("dialog", { name: alert.name });
        await detail
          .getByRole("tab", { name: text("时间线", "Timeline"), exact: true })
          .click();
        await expect(
          detail.locator(".argus-alert-timeline li").first(),
        ).toBeVisible();
        await expect(detail.locator(".argus-alert-timeline pre")).toHaveCount(
          0,
        );
        await page.screenshot({
          path: info.outputPath("service-alert.png"),
          animations: "disabled",
        });
      });
});
