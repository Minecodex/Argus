import { selectWorkbenchNamespace } from "./helpers/kubernetes-workbench";
import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";

test.use({ trace: "off", video: "off", reducedMotion: "reduce" });

type ResourceMap = {
  cluster: { id: string; name: string };
  namespaces: Record<string, { id: string }>;
  application: { id: string };
  dashboards: Record<string, { id: string }>;
  alert_policies: { replicas: { id: string; name: string } };
};

test.describe("Plan9 B09a real Kubernetes monitoring", () => {
  test.skip(
    process.env.ARGUS_PLAN9_B09A_E2E !== "1",
    "Requires the owned running B09a cluster",
  );
  test.describe.configure({ timeout: 180_000 });
  for (const locale of ["zh-CN", "en-US"])
    for (const theme of ["light", "dark"])
      test(locale + " " + theme, async ({ context, page }, info) => {
        const text = (zh: string, en: string) => (locale === "zh-CN" ? zh : en);
        const map: ResourceMap = JSON.parse(
          readFileSync(process.env.ARGUS_PLAN9_RESOURCE_MAP!, "utf8"),
        );
        const session = JSON.parse(
          readFileSync(process.env.ARGUS_PLAN9_SESSION_PATH!, "utf8"),
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

        await page.goto("/kubernetes/" + map.cluster.id);
        await expect(
          page.getByRole("heading", { name: new RegExp(map.cluster.name) }),
        ).toBeVisible();
        await selectWorkbenchNamespace(page, "b09a-workloads");
        await expect(
          page.getByRole("row").filter({
            has: page.getByText("orders", { exact: true }),
          }),
        ).toBeVisible();

        if (locale === "zh-CN" && theme === "light") {
          await page.getByRole("tab", { name: "Pod", exact: true }).click();
          const rows = page.getByRole("row").filter({
            has: page.getByText(/^orders-/),
          });
          await expect(rows.first()).toBeVisible();
          await rows
            .first()
            .getByRole("button", { name: "查看日志", exact: true })
            .click();
          const dialog = page.getByRole("dialog", { name: /^Pod 日志/ });
          await expect(dialog.getByRole("log")).toContainText(
            "p9-b09a-20261108-01",
          );
          await page.screenshot({
            path: info.outputPath("live-pod-logs.png"),
            animations: "disabled",
          });
          await page.keyboard.press("Escape");
        }

        for (const [key, kind, resource] of [
          ["kubernetes.cluster", "kubernetes_cluster", map.cluster.id],
          ["kubernetes.nodes", "kubernetes_cluster", map.cluster.id],
          [
            "workloads",
            "kubernetes_namespace",
            map.namespaces["b09a-workloads"]!.id,
          ],
          ["logs", "application", map.application.id],
        ]) {
          const board = map.dashboards[key!]!.id;
          const executed = page.waitForResponse(
            (r) =>
              r.url().includes("/dashboards/" + board + "/execute") &&
              r.request().method() === "POST",
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
          const value = await response.json();
          expect(value.panels.length).toBeGreaterThan(0);
          expect(
            value.panels.every(
              (p: { status: string }) => p.status === "success",
            ),
          ).toBe(true);
          await expect(page.locator("[data-panel-id]")).toHaveCount(
            value.panels.length,
          );
          if (key === "logs") {
            const panel = page.locator('[data-panel-id="logs"]');
            await expect(panel.getByRole("article").first()).toContainText(
              "p9-b09a-20261108-01",
            );
            await panel
              .getByRole("article")
              .first()
              .getByText(text("完整记录与字段", "Full record and fields"), {
                exact: true,
              })
              .click();
            await expect(panel.getByRole("article").first()).toContainText(
              "source_id",
            );
          }
          expect(
            await page.evaluate(
              () => document.documentElement.scrollWidth <= innerWidth + 1,
            ),
          ).toBe(true);
          await page.screenshot({
            path: info.outputPath(key + ".png"),
            animations: "disabled",
          });
        }

        const alert = map.alert_policies.replicas;
        await page.goto("/alerts?tab=history&policy_id=" + alert.id);
        await page.getByText(alert.name, { exact: true }).first().click();
        const dialog = page.getByRole("dialog", {
          name: alert.name,
          exact: true,
        });
        await dialog
          .getByRole("tab", { name: text("时间线", "Timeline"), exact: true })
          .click();
        await expect(
          dialog.locator(".argus-alert-timeline li").first(),
        ).toBeVisible();
        expect(
          await dialog.locator(".argus-alert-timeline li").count(),
        ).toBeGreaterThanOrEqual(2);
        await expect(dialog.locator(".argus-alert-timeline pre")).toHaveCount(
          0,
        );
        await page.screenshot({
          path: info.outputPath("replica-alert.png"),
          animations: "disabled",
        });
      });
});
