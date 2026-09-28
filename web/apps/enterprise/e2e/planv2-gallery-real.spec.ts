import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import type { DashboardSchemas } from "@argus/api-client";
import { metricObservations } from "../../../packages/ui/src/observation-data";
import { createMfaLogin } from "./helpers/mfa-login";

const login = createMfaLogin("enterprise");
for (const [locale, theme] of [
  ["zh-CN", "light"],
  ["en-US", "dark"],
]) {
  test(`eleven real Metrics charts preserve values, units and histogram buckets ${locale} ${theme}`, async ({
    page,
  }, info) => {
    test.skip(
      process.env.ARGUS_PLANV2_E2E !== "1",
      "Requires real Kubernetes data",
    );
    test.setTimeout(180000);
    await page.addInitScript(
      ({ locale, theme }) => {
        localStorage.setItem("argus.locale", locale!);
        localStorage.setItem("argus.theme", theme!);
      },
      { locale, theme },
    );
    await login(
      page,
      "/login",
      process.env.ARGUS_PLANV2_USERNAME!,
      process.env.ARGUS_PLANV2_PASSWORD!,
    );
    const board = process.env.ARGUS_PLANV2_GALLERY_ID!;
    const response = page.waitForResponse(
      (r) =>
        r.url().endsWith(`/dashboards/${board}/execute`) && r.status() === 200,
    );
    await page.goto(
      `/dashboards/${board}?resource=${process.env.ARGUS_PLANV2_CLUSTER_ID!}`,
    );
    const result = (await (
      await response
    ).json()) as DashboardSchemas["DashboardExecution"];
    expect(result.panels).toHaveLength(11);
    expect(result.partial).toBe(false);
    await expect(page.locator("[data-panel-id]")).toHaveCount(11);
    for (const panel of result.panels) {
      expect(panel.status, panel.id).toBe("success");
      expect(panel.sources.length, panel.id).toBeGreaterThan(0);
      expect(panel.sources.every((s) => s.type === "otlp")).toBe(true);
      const series = metricObservations(panel.targets[0]!.data);
      expect(series.length, panel.id).toBeGreaterThan(0);
      const card = page.locator(`[data-panel-id="${panel.id}"]`);
      await card.scrollIntoViewIfNeeded();
      if (panel.id === "histogram") {
        expect(
          Object.fromEntries(
            series.map((s) => [s.labels.le, s.points.at(-1)![1]]),
          ),
        ).toEqual({ "1": 2, "5": 5, "+Inf": 6 });
      } else {
        expect(series).toHaveLength(1);
        expect(series[0]!.points.at(-1)![1]).toBeCloseTo(
          panel.id === "stat" ? 0.7 : 7,
        );
      }
      if (panel.id === "stat") await expect(card).toContainText("70.0%");
      else if (panel.id === "table")
        await expect(
          card.getByRole("cell", { name: "7.0 ms", exact: true }),
        ).toBeVisible();
      else {
        await expect(
          card.getByRole("img", { name: `Real ${panel.id}`, exact: true }),
        ).toBeVisible();
        const layers = card.locator("canvas");
        await expect.poll(() => layers.count()).toBeGreaterThan(0);
        for (const layer of await layers.all())
          await expect(layer).toBeVisible();
      }
      await card.screenshot({
        path: info.outputPath(`${panel.id}.png`),
        animations: "disabled",
      });
    }
    const report = await new AxeBuilder({ page }).analyze();
    expect(
      report.violations.filter(
        (v) => v.impact === "serious" || v.impact === "critical",
      ),
    ).toEqual([]);
    await page
      .getByRole("heading", {
        name: "PlanV2 real Metrics gallery",
        exact: true,
      })
      .scrollIntoViewIfNeeded();
    await page.screenshot({
      path: info.outputPath("metric-gallery.png"),
      fullPage: true,
      animations: "disabled",
    });
  });
}
