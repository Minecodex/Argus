import { expect, test } from "@playwright/test";
import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { openSealedTool } from "./helpers/sealed-dashboard";

const fixtures = JSON.parse(
  readFileSync(
    resolve(process.cwd(), "../template-runtime/src/fixtures.generated.json"),
    "utf8",
  ),
) as { bindings: { Tool: string; Data: Record<string, unknown> }[] };
const data = fixtures.bindings.find(
  (binding) => binding.Tool === "observe.catalog.query",
)!.Data;

for (const english of [false, true])
  for (const dark of [false, true])
    for (const width of [1280, 1440, 1920])
      test(`Histogram methods ${english ? "en" : "zh"} ${dark ? "dark" : "light"} ${width}`, async ({
        page,
      }, info) => {
        await page.setViewportSize({ width, height: 900 });
        const { content } = await openSealedTool(
          page,
          english,
          dark,
          "observe.catalog.query",
          data,
        );
        for (const [name, representation, expression] of [
          [
            "request_duration",
            english ? "Classic histogram" : "经典直方图",
            '{__name__="request_duration_count"}',
          ],
          [
            "request_size",
            english ? "Native histogram" : "原生直方图",
            'histogram_count({__name__="request_size"})',
          ],
        ] as const) {
          await content
            .getByRole("row")
            .filter({ hasText: name })
            .getByRole("button", {
              name: english ? "Expand" : "展开",
              exact: true,
            })
            .click();
          await content
            .locator("details>summary")
            .filter({ hasText: english ? /^Query methods$/ : /^查询方式$/ })
            .last()
            .click();
          await expect(
            content.getByText(representation, { exact: true }),
          ).toBeVisible();
          await expect(
            content.getByText(expression, { exact: true }),
          ).toBeVisible();
        }
        await expect(
          content.getByText("{observation}", { exact: true }).last(),
        ).toBeVisible();
        const meanUnit = content
          .getByText(english ? "Mean unit" : "均值单位", { exact: true })
          .last();
        await expect(meanUnit).toBeVisible();
        const stream = page.locator(".argus-chat__stream");
        const before = (await meanUnit.boundingBox())!;
        const viewport = (await stream.boundingBox())!;
        await stream.evaluate(
          (element, delta) =>
            element.scrollTo({
              top: element.scrollTop + delta,
              behavior: "instant",
            }),
          before.y - viewport.y - viewport.height / 2,
        );
        await page.evaluate(
          () =>
            new Promise<void>((resolve) =>
              requestAnimationFrame(() =>
                requestAnimationFrame(() => resolve()),
              ),
            ),
        );
        const measured = (await meanUnit.boundingBox())!;
        const composer = (await page
          .locator(".argus-chat-composer")
          .boundingBox())!;
        expect(measured.y).toBeGreaterThanOrEqual(viewport.y);
        expect(measured.y + measured.height).toBeLessThanOrEqual(
          Math.min(viewport.y + viewport.height, composer.y) + 1,
        );
        expect(
          await content
            .locator("body")
            .evaluate((body) => body.scrollWidth <= body.clientWidth + 1),
        ).toBe(true);
        await page.screenshot({
          path: info.outputPath("histogram-methods.png"),
        });
        writeFileSync(
          info.outputPath("receipt.json"),
          JSON.stringify({
            passed: true,
            english,
            dark,
            width,
            classic_and_native_distinct: true,
            result_count_and_mean_units_visible: true,
            mean_unit_pixels_above_composer: true,
            controlled_production_template: true,
            real_model_claimed: false,
          }),
        );
      });
