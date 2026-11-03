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
      test(`Metric quantity ${english ? "en" : "zh"} ${dark ? "dark" : "light"} ${width}`, async ({
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
        const row = content
          .getByRole("row")
          .filter({ hasText: "system_memory_utilization" });
        await expect(row).toContainText("gauge");
        await expect(row).toContainText("1");
        await row
          .getByRole("button", {
            name: english ? "Expand" : "展开",
            exact: true,
          })
          .click();
        await content
          .locator("details>summary")
          .filter({ hasText: english ? /^Quantity semantics$/ : /^量值语义$/ })
          .click();
        const suggestion = content.getByText(
          english ? "Percent (raw × 100)" : "百分比（原值 × 100）",
          { exact: true },
        );
        await expect(suggestion).toBeVisible();
        await expect(
          content.getByText(english ? "Ratio" : "比率", { exact: true }),
        ).toBeVisible();
        const stream = page.locator(".argus-chat__stream");
        const before = (await suggestion.boundingBox())!;
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
        const bounds = (await suggestion.boundingBox())!;
        const composer = (await page
          .locator(".argus-chat-composer")
          .boundingBox())!;
        expect(bounds.y).toBeGreaterThanOrEqual(viewport.y);
        expect(bounds.y + bounds.height).toBeLessThanOrEqual(composer.y);
        expect(
          await content
            .locator("body")
            .evaluate((body) => body.scrollWidth <= body.clientWidth + 1),
        ).toBe(true);
        await page.screenshot({ path: info.outputPath("metric-quantity.png") });
        writeFileSync(
          info.outputPath("receipt.json"),
          JSON.stringify({
            passed: true,
            english,
            dark,
            width,
            raw_unit_unchanged: true,
            source_qualified_ratio: true,
            suggestion_above_composer: true,
            controlled_production_template: true,
            real_model_claimed: false,
          }),
        );
      });
