import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
const real = process.env.ARGUS_KWR_REAL === "1";
for (const width of [1280, 1440, 1920])
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`overview spacing and readable graph ${width} ${english ? "en" : "zh"} ${theme}`, async ({
        page,
      }, info) => {
        test.setTimeout(120000);
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        await page.setViewportSize({ width, height: 1080 });
        await page.addInitScript(
          ({ english, theme }) => {
            localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
            localStorage.setItem("argus.theme", theme);
          },
          { english, theme },
        );
        await page.goto("/login");
        await page
          .locator('input[autocomplete="username"]')
          .fill(real ? "kakj" : "root");
        await page
          .locator('input[autocomplete="current-password"]')
          .fill(real ? process.env.ARGUS_KWR_PASSWORD! : "123456");
        await page.locator('form button[type="submit"]').click();
        await expect(page).not.toHaveURL(/\/login/);
        await page.goto(
          "/kubernetes/" +
            (real ? process.env.ARGUS_KWR_CLUSTER! : "k8s-prod-east"),
        );
        await expect(page.locator(".argus-resource-summary")).toBeVisible({
          timeout: 45000,
        });
        await expect(
          page.locator(".argus-resource-summary__item").first(),
        ).toContainText(/\d/);
        const summary = page
          .locator(".argus-dashboard-tile")
          .filter({ has: page.locator(".argus-resource-summary") });
        const fit = await summary.evaluate((tile) => {
          const bounds = tile.getBoundingClientRect(),
            time = tile
              .querySelector(".argus-k8s-observation-time")!
              .getBoundingClientRect(),
            card = tile
              .querySelector(".argus-resource-summary__item")!
              .getBoundingClientRect();
          return {
            timeInset: bounds.bottom - time.bottom,
            cardInset: card.left - bounds.left,
          };
        });
        expect(fit.timeInset).toBeGreaterThanOrEqual(8);
        expect(fit.cardInset).toBeGreaterThanOrEqual(16);
        const gutter = await page
          .locator(".argus-dashboard-tile")
          .evaluateAll((tiles) => {
            const first = tiles[0]!.getBoundingClientRect(),
              second = tiles[1]!.getBoundingClientRect(),
              third = tiles[2]!.getBoundingClientRect();
            return {
              x: second.left - first.right,
              y: third.top - first.bottom,
            };
          });
        expect(gutter.x).toBeGreaterThanOrEqual(12);
        expect(gutter.y).toBeGreaterThanOrEqual(12);
        await page.screenshot({
          path: info.outputPath("overview-summary.png"),
          fullPage: true,
        });
        const graph = page.locator(".argus-resource-graph");
        await graph.scrollIntoViewIfNeeded();
        await expect(
          graph.locator(".argus-resource-graph__node").first(),
        ).toBeVisible();
        const rectangles = await graph
          .locator(".argus-resource-graph__node")
          .evaluateAll((nodes) =>
            nodes.map((node) => {
              const r = node.getBoundingClientRect();
              return {
                x: r.x,
                y: r.y,
                right: r.right,
                bottom: r.bottom,
                width: r.width,
                height: r.height,
              };
            }),
          );
        expect(rectangles.length).toBeGreaterThan(1);
        for (let i = 0; i < rectangles.length; i++) {
          const a = rectangles[i]!;
          expect(a.width).toBeGreaterThanOrEqual(160);
          expect(a.height).toBeGreaterThanOrEqual(80);
          for (let j = i + 1; j < rectangles.length; j++) {
            const b = rectangles[j]!;
            expect(
              a.x < b.right &&
                a.right > b.x &&
                a.y < b.bottom &&
                a.bottom > b.y,
            ).toBe(false);
          }
        }
        await page.screenshot({
          path: info.outputPath("resource-graph.png"),
          fullPage: true,
        });
        const inactive = graph.getByRole("button", {
          name: /显示历史副本|Show inactive resources/,
        });
        if (await inactive.count()) {
          const count = rectangles.length;
          await inactive.click();
          await expect
            .poll(() => graph.locator(".argus-resource-graph__node").count())
            .toBeGreaterThan(count);
          await page.screenshot({
            path: info.outputPath("resource-graph-with-history.png"),
            fullPage: true,
          });
        }
        await page
          .locator(".argus-dashboard-tile")
          .last()
          .scrollIntoViewIfNeeded();
        await page.screenshot({
          path: info.outputPath("resource-tables.png"),
          fullPage: true,
        });
        const violations = (
          await new AxeBuilder({ page }).analyze()
        ).violations.filter(
          (issue) => issue.impact === "serious" || issue.impact === "critical",
        );
        expect(violations).toEqual([]);
        expect(errors).toEqual([]);
      });
    }
