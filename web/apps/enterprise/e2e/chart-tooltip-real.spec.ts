import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";

test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
const cluster = "01a1b8f7-875a-721d-8084-3c8e6c80a5cc";
const board = "df0a6a46-cbdc-54d9-a4d0-41ac509368db";
type Execution = {
  panels: {
    id: string;
    targets: { data?: { metric: Record<string, string> }[] }[];
  }[];
};

test.describe("Deployed chart hover with real component series", () => {
  test.skip(
    process.env.ARGUS_CHART_TOOLTIP_E2E !== "1",
    "Requires the owned live component dashboard and normal login",
  );
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`${english ? "en" : "zh"} ${theme}`, async ({
        context,
        page,
      }, info) => {
        test.setTimeout(90_000);
        const session = JSON.parse(
          readFileSync(process.env.ARGUS_CHART_TOOLTIP_SESSION!, "utf8"),
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
          height: 800,
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
        const executing = page.waitForResponse(
          (response) =>
            new URL(response.url()).pathname.endsWith(
              `/dashboards/${board}/execute`,
            ) && response.request().method() === "POST",
        );
        await page.goto(
          `/dashboards/${board}?resource=${cluster}&resource_type=kubernetes_cluster`,
        );
        const response = await executing;
        expect(response.status()).toBe(200);
        const result: Execution = await response.json();
        const data = result.panels
          .find((p) => p.id === "depth")!
          .targets.flatMap((t) => t.data ?? []);
        expect(data.length).toBeGreaterThan(20);
        const queueNames = data.map((row) => row.metric.name).filter(Boolean);
        const panel = page.locator('[data-panel-id="depth"]');
        const chart = panel.getByRole("img");
        await chart.scrollIntoViewIfNeeded();
        const box = (await chart.boundingBox())!;
        await page.mouse.move(box.x + box.width * 0.6, box.y + box.height / 2);
        const tip = page
          .locator(".argus-chart-tooltip")
          .filter({ visible: true })
          .last();
        await expect(tip).toBeVisible();
        await expect(tip).toHaveCSS("opacity", "1");
        const bounds = (await tip.boundingBox())!,
          viewport = page.viewportSize()!;
        expect(bounds.width).toBeLessThanOrEqual(441);
        expect(bounds.height).toBeLessThanOrEqual(321);
        expect(bounds.x).toBeGreaterThanOrEqual(0);
        expect(bounds.y).toBeGreaterThanOrEqual(0);
        expect(bounds.x + bounds.width).toBeLessThanOrEqual(viewport.width);
        expect(bounds.y + bounds.height).toBeLessThanOrEqual(viewport.height);
        const labels = await tip
          .locator(".argus-chart-tooltip__name")
          .allTextContents();
        expect(labels.length).toBeGreaterThan(20);
        expect(
          labels.every((name) =>
            queueNames.some((queue) => name.includes(queue)),
          ),
        ).toBe(true);
        expect(
          labels.some((name) => /[a-f0-9]{8}-[a-f0-9]{4}-/i.test(name)),
        ).toBe(false);
        expect(labels.every((name) => name.length < 160)).toBe(true);
        const list = tip.getByRole("list");
        expect(
          await list.evaluate((el) => el.scrollHeight > el.clientHeight),
        ).toBe(true);
        await list.hover();
        await page.mouse.wheel(0, 450);
        await expect
          .poll(() => list.evaluate((el) => el.scrollTop))
          .toBeGreaterThan(0);
        await page.screenshot({
          path: info.outputPath("real-bounded-tooltip.png"),
        });
        await page.mouse.move(2, 2);
        await expect(tip).not.toBeVisible();
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth,
          ),
        ).toBe(true);
        expect(errors).toEqual([]);
      });
    }
});
