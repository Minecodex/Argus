import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";

test.use({ trace: "off", video: "off" });
test.describe("Deployed Kubernetes collection controls and dashboards", () => {
  test.skip(
    process.env.ARGUS_KUBERNETES_COLLECTION_E2E !== "1",
    "Requires the owned B09a deployment and a normal enterprise session",
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
          height: 1100,
        });
        await page.addInitScript(
          ({ english, theme }) => {
            localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
            localStorage.setItem("argus.theme", theme);
          },
          { english, theme },
        );
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        const cluster = "01a1b8f7-875a-721d-8084-3c8e6c80a5cc";
        await page.goto(`/kubernetes/${cluster}#otlp-collector`);
        const panel = page.getByRole("tabpanel", {
          name: text("采集配置", "Collection"),
          exact: true,
        });
        await expect(
          panel.getByRole("table").getByRole("columnheader"),
        ).toHaveCount(4);
        await expect(
          panel.getByRole("columnheader", {
            name: text("开启采集", "Collect"),
            exact: true,
          }),
        ).toBeVisible();
        await expect(panel.getByRole("switch")).toHaveCount(8);
        for (const name of [
          text("集群与工作负载状态", "Cluster and workload state"),
          text("节点与容器指标", "Node and container metrics"),
          text("Collector 自身指标", "Collector metrics"),
        ]) {
          const row = panel
            .getByRole("row")
            .filter({ has: page.getByText(name, { exact: true }) });
          await expect(row.getByRole("switch")).toBeChecked();
          await expect(row).toContainText(text("数据新鲜", "Fresh"));
        }
        await expect(panel.getByRole("textbox")).toHaveCount(0);
        await expect(panel.getByRole("progressbar")).toHaveCount(0);
        await expect(
          panel.getByText(/Claim|Revision|Digest|Profile/),
        ).toHaveCount(0);
        await page.getByRole("heading", { level: 1 }).click();
        await panel.screenshot({
          path: info.outputPath("collection-controls.png"),
        });
        const dashboards = [
          "1764d344-bb5a-5b16-9f09-13f68a452bc9",
          "c17f3435-0dc8-544f-bef4-e5e71c3bc381",
          "258c6833-6dce-5240-bddf-81bbf7c6f2c2",
        ];
        for (const id of dashboards) {
          const execution = page.waitForResponse(
            (response) =>
              new URL(response.url()).pathname.endsWith(
                `/dashboards/${id}/execute`,
              ) && response.request().method() === "POST",
          );
          await page.goto(
            `/dashboards/${id}?resource=${cluster}&resource_type=kubernetes_cluster`,
          );
          const response = await execution;
          expect(response.status()).toBe(200);
          const data = await response.json();
          expect(data.panels.length).toBeGreaterThan(0);
          for (const result of data.panels) {
            expect(result.status).toBe("success");
            expect(
              result.targets.some(
                (target: { data?: unknown[] }) =>
                  (target.data?.length ?? 0) > 0,
              ),
            ).toBe(true);
          }
          await expect(
            page.getByText(text("暂无数据", "No data"), { exact: true }),
          ).toHaveCount(0);
          await page.screenshot({
            path: info.outputPath(`${id}.png`),
            fullPage: true,
          });
        }
        expect(errors).toEqual([]);
      });
    }
});
