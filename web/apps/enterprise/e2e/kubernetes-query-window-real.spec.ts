import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
const cluster = "01a1b8f7-875a-721d-8084-3c8e6c80a5cc";
const boards = {
  nodes: "c17f3435-0dc8-544f-bef4-e5e71c3bc381",
  apiserver: "ba89e0b8-0204-5461-84a9-1931aed51b58",
};
type Result = {
  panels: {
    status: string;
    targets: {
      status: string;
      meta: { scanned_bytes: number };
      data?: { values?: [number, string][]; value?: [number, string] }[];
    }[];
  }[];
};

test.describe("Deployed Kubernetes one-hour queries", () => {
  test.skip(
    process.env.ARGUS_KUBERNETES_QUERY_E2E !== "1",
    "Requires the owned live cluster and a normal enterprise session",
  );
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`${english ? "en" : "zh"} ${theme}`, async ({
        context,
        page,
      }, info) => {
        test.setTimeout(120_000);
        const session = JSON.parse(
          readFileSync(process.env.ARGUS_KUBERNETES_QUERY_SESSION!, "utf8"),
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
        const failures: string[] = [];
        page.on("pageerror", (error) => failures.push(error.message));
        for (const [name, id] of Object.entries(boards)) {
          const executing = page.waitForResponse(
            (response) =>
              new URL(response.url()).pathname.endsWith(
                `/dashboards/${id}/execute`,
              ) && response.request().method() === "POST",
          );
          const started = Date.now();
          await page.goto(
            `/dashboards/${id}?resource=${cluster}&resource_type=kubernetes_cluster`,
          );
          const response = await executing;
          expect(response.status()).toBe(200);
          const request = response.request().postDataJSON() as {
            from: string;
            to: string;
          };
          expect(Date.parse(request.to) - Date.parse(request.from)).toBe(
            3_600_000,
          );
          const result: Result = await response.json();
          expect(Date.now() - started).toBeLessThan(10_000);
          expect(result.panels).toHaveLength(6);
          let bytes = 0;
          for (const panel of result.panels) {
            expect(panel.status).toBe("success");
            expect(
              panel.targets.some(
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
            for (const target of panel.targets)
              bytes += target.meta.scanned_bytes;
          }
          expect(bytes).toBeLessThanOrEqual(256 * 1024 * 1024);
          await expect(page.getByText(/QUERY_BUDGET_EXCEEDED/)).toHaveCount(0);
          await expect(
            page.getByText(english ? "No data" : "暂无数据", { exact: true }),
          ).toHaveCount(0);
          if (name === "apiserver") {
            await expect(
              page.getByText(english ? "Template version 2" : "模板版本 2", {
                exact: true,
              }),
            ).toBeVisible();
            await expect(
              page
                .getByText(
                  english
                    ? "API mean request duration (excluding long connections)"
                    : "API 平均请求耗时（不含长连接）",
                  { exact: true },
                )
                .first(),
            ).toBeVisible();
            const detail = await page.request.get(`/api/v1/dashboards/${id}`);
            const body = (await detail.json()) as {
              revision: { spec: { panels: { id: string; unit: string }[] } };
            };
            for (const panel of body.revision.spec.panels.filter((p) =>
              ["api_latency", "api_p95"].includes(p.id),
            ))
              expect(panel.unit).toBe("ms");
          }
          expect(
            (await new AxeBuilder({ page }).analyze()).violations.filter(
              (v) => v.impact === "critical" || v.impact === "serious",
            ),
          ).toEqual([]);
          await page.screenshot({
            path: info.outputPath(`${name}-one-hour.png`),
            fullPage: true,
          });
          await info.attach(`${name}-metrics`, {
            body: JSON.stringify({
              duration_ms: Date.now() - started,
              scanned_bytes: bytes,
            }),
            contentType: "application/json",
          });
        }
        expect(failures).toEqual([]);
      });
    }
});
