import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";
import type {
  DashboardDraft,
  DashboardSpec,
} from "../../../packages/api-client/src/dashboard";

test.use({
  trace: "off",
  video: "off",
  actionTimeout: 15000,
  viewport: { width: 1440, height: 1000 },
});

test.describe("real dashboard metric catalog", () => {
  test.skip(
    !process.env.ARGUS_CATALOG_SESSION_PATH,
    "requires a real session and collected host metrics",
  );
  test.setTimeout(90_000);

  for (const locale of ["zh-CN", "en-US"])
    for (const theme of ["light", "dark"])
      test(`${locale} ${theme}: discover, select, save and run another metric`, async ({
        page,
        context,
      }, info) => {
        const session = JSON.parse(
          readFileSync(process.env.ARGUS_CATALOG_SESSION_PATH!, "utf8"),
        );
        await context.addCookies(
          session.cookies.map((cookie: object) => ({
            ...cookie,
            httpOnly: true,
            sameSite: "Lax",
          })),
        );
        await page.addInitScript(
          ({ locale, theme }) => {
            localStorage.setItem("argus.locale", locale);
            localStorage.setItem("argus.theme", theme);
          },
          { locale, theme },
        );
        const headers = {
          Origin: session.origin,
          "X-CSRF-Token": session.csrf,
        };
        if (process.env.ARGUS_CATALOG_PAGINATE === "1") {
          await page.route("**/dashboards/catalog/query", async (route) => {
            const body = route.request().postDataJSON();
            if (body.kind !== "metrics" || body.metric) return route.continue();
            // Exercise real signed cursors with the collected metric count.
            // Only reduce the requested page size; responses remain real.
            const response = await route.fetch({
              postData: JSON.stringify({ ...body, limit: 15 }),
            });
            await route.fulfill({ response });
          });
        }
        const listing = await context.request.get("/api/v1/dashboards");
        expect(listing.status()).toBe(200);
        const host = (await listing.json()).find(
          (item: { name: string }) => item.name === "Host",
        );
        expect(host).toBeTruthy();
        const board = await context.request.get(
          `/api/v1/dashboards/${host.id}`,
        );
        expect(board.status()).toBe(200);
        const spec = (await board.json()).revision.spec as DashboardSpec;
        const panel = spec.panels.find((item) => item.id === "cpu")!;
        spec.panels = [panel];
        spec.default_refresh_seconds = 0;
        panel.authoring_mode = "builder";
        panel.title = "Catalog regression";
        panel.targets[0].source_definition = {
          builder: {
            operation: "value",
            metric: "system_cpu_utilization",
            filters: [],
            group_by: [],
          },
        };
        const created = await context.request.post("/api/v1/dashboard-drafts", {
          headers,
          data: {
            name: `Catalog regression ${locale} ${theme}`,
            description: "Temporary E2E draft",
            spec,
            proposed_bindings: [],
          },
        });
        expect(created.status()).toBe(201);
        const draft = (await created.json()) as DashboardDraft;
        const path = `/api/v1/dashboard-drafts/${draft.id}`;
        const catalogFailures: number[] = [];
        page.on("response", (response) => {
          if (
            response.url().endsWith("/dashboards/catalog/query") &&
            response.status() >= 400
          )
            catalogFailures.push(response.status());
        });
        try {
          await page.goto(`/dashboard-drafts/${draft.id}/panels/${panel.id}`);
          const metric = page.getByRole("combobox", {
            name: locale === "zh-CN" ? /^指标\*?$/ : /^Metric\*?$/,
          });
          await expect(metric).toHaveValue("system_cpu_utilization");
          // Clear the search to exercise the full metric directory, then search
          // and select a different observed metric through the real dropdown.
          const directory = page.waitForResponse(
            (response) =>
              response.url().endsWith("/dashboards/catalog/query") &&
              response.request().postDataJSON().kind === "metrics" &&
              response.request().postDataJSON().search === "",
          );
          await metric.fill("");
          const loaded = await directory;
          expect(loaded.status()).toBe(200);
          expect((await loaded.json()).metrics.length).toBeGreaterThan(1);
          await expect(
            page.getByRole("button", { name: /加载更多|Load more/ }),
          ).toHaveCount(0);
          if (process.env.ARGUS_CATALOG_PAGINATE === "1") {
            expect((await loaded.json()).next_cursor).toBeTruthy();
            const next = page.waitForResponse(
              (response) =>
                response.url().endsWith("/dashboards/catalog/query") &&
                !!response.request().postDataJSON().cursor &&
                response.request().postDataJSON().search === "",
              { timeout: 30000 },
            );
            await metric.press("ArrowDown");
            await page
              .locator(".argus-combobox__popover")
              .evaluate(async (el) => {
                await Promise.all(
                  el
                    .getAnimations({ subtree: true })
                    .filter((a) =>
                      Number.isFinite(a.effect?.getComputedTiming().endTime),
                    )
                    .map((a) => a.finished.catch(() => undefined)),
                );
              });
            await page.getByRole("listbox").hover();
            await page.mouse.wheel(0, 2000);
            const second = await next;
            expect(second.status()).toBe(200);
            expect(second.request().postDataJSON()).toMatchObject({
              from: loaded.request().postDataJSON().from,
              to: loaded.request().postDataJSON().to,
            });
            expect((await second.json()).metrics.length).toBeGreaterThan(0);
          }
          const search = page.waitForResponse(
            (response) =>
              response.url().endsWith("/dashboards/catalog/query") &&
              response.request().postDataJSON().search === "system_memory",
          );
          await metric.fill("system_memory");
          expect((await search).status()).toBe(200);
          const option = page.getByRole("option", {
            name: /^system_memory_utilization/,
          });
          await expect(option).toBeVisible();
          await page.screenshot({
            path: info.outputPath("metric-options.png"),
          });
          const metadata = page.waitForResponse(
            (response) =>
              response.url().endsWith("/dashboards/catalog/query") &&
              response.request().postDataJSON().metric ===
                "system_memory_utilization",
            { timeout: 30000 },
          );
          await option.click();
          await expect(metric).toHaveValue("system_memory_utilization");
          await page.keyboard.press("Escape");
          expect((await metadata).status()).toBe(200);
          await expect(
            page.getByText(locale === "zh-CN" ? "草稿已保存" : "Draft saved"),
          ).toBeVisible();
          const result = page.waitForResponse(
            (response) => response.url().endsWith(`${path}/sample`),
            { timeout: 30000 },
          );
          await page
            .getByRole("button", {
              name: locale === "zh-CN" ? "运行查询" : "Run query",
              exact: true,
            })
            .click();
          const executed = await result;
          expect(executed.status()).toBe(200);
          const output = await executed.json();
          expect(output.sample.execution.panels[0].status).toBe("success");
          const target = output.sample.execution.panels[0].targets[0];
          const series = Array.isArray(target.data)
            ? target.data
            : target.data.result;
          expect(series.length).toBeGreaterThan(0);
          const stored = await context.request.get(path);
          expect(stored.status()).toBe(200);
          expect(
            (await stored.json()).spec.panels[0].targets[0].source_definition
              .builder.metric,
          ).toBe("system_memory_utilization");
          await page.reload();
          await expect(metric).toHaveValue("system_memory_utilization");
          expect(catalogFailures).toEqual([]);
          await info.attach("catalog-result", {
            body: JSON.stringify({
              metrics: (await loaded.json()).metrics.length,
              catalogFailures,
              series: series.length,
            }),
            contentType: "application/json",
          });
        } finally {
          await page.goto("about:blank");
          const current = await context.request.get(path);
          expect(current.status()).toBe(200);
          const removed = await context.request.delete(path, {
            headers,
            data: { expected_version: (await current.json()).draft_version },
          });
          expect(removed.ok()).toBe(true);
          const discarded = await context.request.get(path);
          expect(discarded.status()).toBe(200);
          expect((await discarded.json()).status).toBe("discarded");
        }
      });
});
