import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
const cluster = process.env.ARGUS_KWR_CLUSTER ?? "";
test.describe("KWR deployed overview", () => {
  test.skip(
    process.env.ARGUS_KWR_REAL !== "1",
    "Requires the deployed real API and normal account",
  );
  for (const [locale, theme, width] of [
    ["zh-CN", "light", 1280],
    ["en-US", "dark", 1440],
    ["zh-CN", "dark", 1920],
    ["en-US", "light", 1920],
  ] as const) {
    test(`normal login overview and resource drilldown ${locale} ${theme} ${width}`, async ({
      page,
    }, info) => {
      test.setTimeout(120000);
      const english = locale === "en-US",
        errors: string[] = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.setViewportSize({ width, height: 1100 });
      await page.addInitScript(
        ({ locale, theme }) => {
          localStorage.setItem("argus.locale", locale);
          localStorage.setItem("argus.theme", theme);
        },
        { locale, theme },
      );
      await page.goto("/login");
      await page.locator('input[autocomplete="username"]').fill("kakj");
      await page
        .locator('input[autocomplete="current-password"]')
        .fill(process.env.ARGUS_KWR_PASSWORD!);
      await page.locator('form button[type="submit"]').click();
      await expect(page).not.toHaveURL(/login/);
      const response = page.waitForResponse(
        (r) =>
          r.url().endsWith("/overview-dashboard/execute") &&
          r.request().method() === "POST" &&
          r.status() === 200,
      );
      await page.goto("/kubernetes/" + cluster);
      const data = await (await response).json();
      expect(data.kubernetes_scope.cluster_id).toBe(cluster);
      expect(data.panels).toHaveLength(12);
      await expect(
        page.getByRole("tab", {
          name: english ? "Overview" : "大盘",
          exact: true,
        }),
      ).toHaveAttribute("aria-selected", "true");
      await expect(page.locator(".argus-resource-summary")).toBeVisible();
      await expect(page.getByText(/模拟模式|Mock mode/)).toHaveCount(0);
      const health = data.panels.find((p: { id: string }) => p.id === "health");
      expect(health.status).toBe("success");
      expect(health.targets[0].data.counts.node.total).toBe(1);
      for (const id of ["cpu", "memory", "cpu_requests", "memory_requests"]) {
        const panel = data.panels.find((p: { id: string }) => p.id === id);
        expect(panel.status).toBe("success");
        expect(
          panel.targets.some(
            (target: { data: unknown[] }) =>
              Array.isArray(target.data) && target.data.length > 0,
          ),
        ).toBe(true);
      }
      await expect(
        page.locator(".argus-observation-chart canvas").first(),
      ).toBeVisible();
      await page.screenshot({
        path: info.outputPath("real-overview.png"),
        fullPage: true,
      });
      const node = data.panels
        .find((p: { id: string }) => p.id === "nodes")
        .targets[0].data.resources.find(
          (r: { type: string }) => r.type === "node",
        );
      await page
        .getByRole("button", { name: node.name, exact: true })
        .first()
        .click();
      await expect(page).toHaveURL(new RegExp("resource_uid=" + node.uid));
      await expect(
        page.getByRole("dialog", { name: "Node · " + node.name, exact: true }),
      ).toBeVisible();
      await page.screenshot({
        path: info.outputPath("real-node-detail.png"),
        fullPage: true,
      });
      await page.keyboard.press("Escape");
      await expect(
        page.getByRole("tab", {
          name: english ? "Resources" : "资源查询",
          exact: true,
        }),
      ).toHaveAttribute("aria-selected", "true");
      const axe = await new AxeBuilder({ page }).analyze();
      expect(
        axe.violations.filter(
          (v) => v.impact === "critical" || v.impact === "serious",
        ),
      ).toEqual([]);
      expect(errors).toEqual([]);
      await info.attach("actual-panel-status", {
        body: JSON.stringify(
          data.panels.map((p: { id: string; status: string }) => ({
            id: p.id,
            status: p.status,
          })),
        ),
        contentType: "application/json",
      });
    });
  }
});
