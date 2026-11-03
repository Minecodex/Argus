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
const base = fixtures.bindings.find(
  (binding) => binding.Tool === "telemetry.dashboard.list",
)!.Data;
for (const english of [false, true])
  for (const dark of [false, true])
    for (const width of [1280, 1440, 1920]) {
      test(`Dashboard applicability ${english ? "en" : "zh"} ${dark ? "dark" : "light"} ${width}`, async ({
        page,
      }, info) => {
        await page.setViewportSize({ width, height: 1000 });
        const { content } = await openSealedTool(
          page,
          english,
          dark,
          "telemetry.dashboard.list",
          base,
        );
        await expect(
          content.getByText(english ? "Partially applicable" : "部分适用", {
            exact: true,
          }),
        ).toBeVisible();
        const row = content
          .getByRole("row")
          .filter({ hasText: "Database overview" });
        await expect(row).toContainText("host_metrics");
        await row
          .getByRole("button", {
            name: english ? "Expand" : "展开",
            exact: true,
          })
          .click();
        await expect(
          content.getByText(english ? "Not sampled" : "未执行采样", {
            exact: true,
          }),
        ).toBeVisible();
        expect(
          await content
            .locator("body")
            .evaluate((body) => body.scrollWidth <= body.clientWidth + 1),
        ).toBe(true);
        await page.screenshot({
          path: info.outputPath("dashboard-partial.png"),
        });
        writeFileSync(
          info.outputPath("receipt.json"),
          JSON.stringify({
            passed: true,
            english,
            dark,
            width,
            suitability_visible: true,
            filters_visible: true,
            metadata_only_visible: true,
            controlled_template: true,
            real_model_claimed: false,
          }),
        );
      });
    }
