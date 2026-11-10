import { readFileSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

test.use({ trace: "off", video: "off", reducedMotion: "reduce" });

type Collection = {
  id: string;
  desired_enabled: boolean;
  apply_status: string;
  health: string;
  freshness: string;
  last_sample_at: string;
};
type Builtin = { definition: { key: string }; collection: Collection };
type Board = { id: string; name: string; collection: string };
type Series = { values?: [number, string][]; value?: [number, string] };
type Execution = {
  panels: { status: string; targets: { status: string; data?: Series[] }[] }[];
};
const cluster = "01a1b8f7-875a-721d-8084-3c8e6c80a5cc";
const builtinPath = `/api/v1/monitoring/builtins?target_type=kubernetes_cluster&target_id=${cluster}`;
const names: Record<string, [string, string]> = {
  kubernetes_apiserver_metrics: ["API Server 指标", "API server metrics"],
  kubernetes_coredns_metrics: ["CoreDNS 指标", "CoreDNS metrics"],
  kubernetes_scheduler_metrics: ["Scheduler 指标", "Scheduler metrics"],
  kubernetes_controller_metrics: [
    "Controller Manager 指标",
    "Controller Manager metrics",
  ],
  kubernetes_etcd_metrics: ["etcd 组件指标", "etcd component metrics"],
};

async function builtins(page: Page): Promise<Builtin[]> {
  const response = await page.request.get(builtinPath);
  expect(response.status()).toBe(200);
  return response.json();
}

test.describe("B09b deployed component collection and dashboards", () => {
  test.skip(
    process.env.ARGUS_PLAN9_B09B_E2E !== "1",
    "Requires the owned B09b environment and normal login",
  );
  test.describe.configure({ mode: "serial", timeout: 180_000 });
  test.beforeEach(async ({ context }) => {
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
  });

  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`${english ? "en" : "zh"} ${theme}`, async ({ page }, info) => {
        const text = (zh: string, en: string) => (english ? en : zh);
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
        const boards: Record<string, Board> = JSON.parse(
          readFileSync(process.env.ARGUS_PLAN9_COMPONENT_BOARDS!, "utf8"),
        );
        await page.goto(`/kubernetes/${cluster}#otlp-collector`);
        const panel = page.getByRole("tabpanel", {
          name: text("采集配置", "Collection"),
          exact: true,
        });
        await expect(panel.getByRole("switch")).toHaveCount(8);
        const rows = await builtins(page);
        expect(rows).toHaveLength(8);
        for (const row of rows) {
          expect(row.collection.desired_enabled).toBe(true);
          expect(row.collection.health).toBe("healthy");
          expect(row.collection.freshness).toBe("fresh");
          expect(
            Date.now() - Date.parse(row.collection.last_sample_at),
          ).toBeLessThan(120_000);
        }
        await expect(
          panel.getByRole("table").getByRole("columnheader"),
        ).toHaveCount(4);
        await expect(panel.getByRole("textbox")).toHaveCount(0);
        await expect(
          panel.getByText(/Claim|Revision|Digest|Profile/),
        ).toHaveCount(0);
        expect(
          (await new AxeBuilder({ page }).analyze()).violations.filter(
            (v) => v.impact === "critical" || v.impact === "serious",
          ),
        ).toEqual([]);
        await panel.screenshot({
          path: info.outputPath("component-switches.png"),
        });
        for (const [template, board] of Object.entries(boards)) {
          if (!page.url().includes("#otlp-collector"))
            await page.goto(`/kubernetes/${cluster}#otlp-collector`);
          const row = panel.getByRole("row").filter({
            has: page.getByText(names[board.collection]![english ? 1 : 0], {
              exact: true,
            }),
          });
          await expect(row.getByRole("switch")).toBeChecked();
          await expect(row).toContainText(text("数据新鲜", "Fresh"));
          const executing = page.waitForResponse(
            (response) =>
              new URL(response.url()).pathname.endsWith(
                `/dashboards/${board.id}/execute`,
              ) && response.request().method() === "POST",
          );
          await row
            .getByRole("button", {
              name: text("默认仪表盘", "Default dashboard"),
              exact: true,
            })
            .click();
          const response = await executing;
          expect(response.status()).toBe(200);
          const execution: Execution = await response.json();
          expect(execution.panels).toHaveLength(6);
          for (const result of execution.panels) {
            expect(result.status).toBe("success");
            expect(
              result.targets.some(
                (target) =>
                  target.status === "success" &&
                  target.data?.some((series) =>
                    [
                      ...(series.values ?? []),
                      ...(series.value ? [series.value] : []),
                    ].some((point) => Number.isFinite(Number(point[1]))),
                  ),
              ),
            ).toBe(true);
          }
          await expect(page).toHaveURL(
            new RegExp(`/dashboards/${board.id}.*resource=${cluster}`),
          );
          await expect(
            page.getByText(text("暂无数据", "No data"), { exact: true }),
          ).toHaveCount(0);
          await expect(page.getByText(/QUERY_BUDGET_EXCEEDED/)).toHaveCount(0);
          await page.screenshot({
            path: info.outputPath(`${template}.png`),
            fullPage: true,
          });
        }
        await page.goto(`/kubernetes/${cluster}?tab=dashboards`);
        for (const board of Object.values(boards))
          await expect(
            page.locator(`a[href*="/dashboards/${board.id}"]`),
          ).toBeVisible();
        expect(errors).toEqual([]);
      });
    }

  test("CoreDNS stops independently and resumes actual samples through the portal", async ({
    page,
  }) => {
    await page.goto(`/kubernetes/${cluster}#otlp-collector`);
    const row = page
      .getByRole("row")
      .filter({ has: page.getByText("CoreDNS 指标", { exact: true }) });
    const key = "kubernetes_coredns_metrics";
    const confirm = async () => {
      const dialog = page.getByRole("dialog", {
        name: "自身监控配置预览",
        exact: true,
      });
      await dialog.getByRole("button", { name: /^确认/ }).click();
      await expect(dialog).not.toBeVisible();
    };
    try {
      await row.getByRole("switch").focus();
      await row.getByRole("switch").press("Space");
      await confirm();
      await expect
        .poll(
          async () => {
            const collection = (await builtins(page)).find(
              (r) => r.definition.key === key,
            )!.collection;
            return (
              !collection.desired_enabled &&
              collection.apply_status === "effective"
            );
          },
          { timeout: 90_000 },
        )
        .toBe(true);
      await expect(row.getByText("未开启", { exact: true })).toBeVisible({
        timeout: 30_000,
      });
      await expect(
        row.getByRole("button", { name: "默认仪表盘", exact: true }),
      ).toBeVisible();
      const stoppedAt = (await builtins(page)).find(
        (r) => r.definition.key === key,
      )!.collection.last_sample_at;
      const cutoff = Date.now() + 20_000;
      await expect
        .poll(
          async () =>
            (await builtins(page))
              .filter((r) => r.definition.key !== key)
              .every(
                (r) =>
                  r.collection.health === "healthy" &&
                  Date.parse(r.collection.last_sample_at) >= cutoff,
              ),
          { timeout: 60_000 },
        )
        .toBe(true);
      expect(
        (await builtins(page)).find((r) => r.definition.key === key)!.collection
          .last_sample_at,
      ).toBe(stoppedAt);
    } finally {
      if (!(await row.getByRole("switch").isChecked())) {
        await row.getByRole("switch").focus();
        await row.getByRole("switch").press("Space");
        await confirm();
      }
    }
    await expect
      .poll(
        async () =>
          (await builtins(page)).every(
            (r) =>
              r.collection.desired_enabled &&
              r.collection.health === "healthy" &&
              r.collection.freshness === "fresh",
          ),
        { timeout: 90_000 },
      )
      .toBe(true);
    await expect(row).toContainText("数据新鲜", { timeout: 30_000 });
  });
});
