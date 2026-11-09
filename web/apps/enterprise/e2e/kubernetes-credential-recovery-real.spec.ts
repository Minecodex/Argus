import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";
import { selectWorkbenchNamespace } from "./helpers/kubernetes-workbench";

test.use({ trace: "off", video: "off" });

test.describe("Kubernetes credential recovery in the deployed portal", () => {
  test.skip(
    process.env.ARGUS_KUBERNETES_RECOVERY_E2E !== "1",
    "Requires an owned running B09a cluster and a normal enterprise session",
  );
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`${english ? "en" : "zh"} ${theme}`, async ({
        context,
        page,
      }, info) => {
        test.setTimeout(120_000);
        const text = (zh: string, en: string) => (english ? en : zh);
        const session = JSON.parse(
          readFileSync(process.env.ARGUS_KUBERNETES_RECOVERY_SESSION!, "utf8"),
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
          height: 1000,
        });
        await page.addInitScript(
          ({ english, theme }) => {
            localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
            localStorage.setItem("argus.theme", theme);
          },
          { english, theme },
        );
        const errors: string[] = [];
        const watches: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        page.on("request", (request) => {
          if (new URL(request.url()).pathname.endsWith("/inventory/watch"))
            watches.push(request.method());
        });
        await page.goto("/kubernetes/01a1b8f7-875a-721d-8084-3c8e6c80a5cc");
        const workbench = page.locator(".argus-k8s-workbench");
        await expect(workbench).toBeVisible();
        await selectWorkbenchNamespace(page, "b09a-workloads");
        await expect(
          workbench.getByText(
            text(
              "部分扩展资源类型暂时无法发现。",
              "Some extension resource types could not be discovered.",
            ),
            { exact: true },
          ),
        ).toHaveCount(0);
        const types = page.getByRole("tablist", {
          name: text("资源类型", "Resource type"),
          exact: true,
        });
        const categories = page.getByRole("tablist", {
          name: text("资源类别", "Resource category"),
          exact: true,
        });
        for (const kind of [
          "Deployment",
          "Pod",
          "StatefulSet",
          "DaemonSet",
          "Job",
        ]) {
          await types.getByRole("tab", { name: kind, exact: true }).click();
          await expect(workbench.getByRole("table")).toBeVisible();
          await expect
            .poll(() => workbench.getByRole("table").getByRole("row").count())
            .toBeGreaterThan(1);
          await expect(workbench.getByRole("alert")).toHaveCount(0);
        }
        await categories
          .getByRole("tab", { name: text("网络", "Networking"), exact: true })
          .click();
        await types.getByRole("tab", { name: "Service", exact: true }).click();
        await expect
          .poll(() => workbench.getByRole("table").getByRole("row").count())
          .toBeGreaterThan(1);
        await categories
          .getByRole("tab", { name: text("存储", "Storage"), exact: true })
          .click();
        await types
          .getByRole("tab", { name: "PersistentVolumeClaim", exact: true })
          .click();
        await expect
          .poll(() => workbench.getByRole("table").getByRole("row").count())
          .toBeGreaterThan(1);
        await categories
          .getByRole("tab", { name: text("集群", "Cluster"), exact: true })
          .click();
        await types
          .getByRole("tab", { name: "Namespace", exact: true })
          .click();
        await expect(
          workbench
            .getByRole("row")
            .filter({ has: page.getByText("b09a-workloads", { exact: true }) }),
        ).toBeVisible();
        await categories
          .getByRole("tab", {
            name: text("工作负载", "Workloads"),
            exact: true,
          })
          .click();
        await types.getByRole("tab", { name: "Pod", exact: true }).click();
        await expect(workbench.getByText(/^orders-/).first()).toBeVisible();
        const refreshed = page.waitForResponse((response) => {
          const url = new URL(response.url());
          return (
            response.request().method() === "GET" &&
            url.pathname.endsWith("/inventory") &&
            url.searchParams.get("type") === "pod" &&
            response.status() === 200
          );
        });
        await workbench
          .getByRole("button", { name: text("刷新", "Refresh"), exact: true })
          .click();
        await refreshed;
        await expect(workbench.getByRole("alert")).toHaveCount(0);
        await expect(workbench.getByRole("switch")).toHaveCount(0);
        expect(watches).toEqual([]);
        expect(errors).toEqual([]);
        await page.getByRole("heading", { level: 1 }).click();
        await workbench.screenshot({
          path: info.outputPath("recovered-kubernetes.png"),
        });
      });
    }
});
