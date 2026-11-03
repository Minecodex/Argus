import { expect, test } from "@playwright/test";
import { writeFileSync } from "node:fs";
import { openSealedTool } from "./helpers/sealed-dashboard";

for (const english of [false, true])
  for (const dark of [false, true])
    for (const width of [1280, 1440, 1920])
      test(`Incomplete catalog ${english ? "en" : "zh"} ${dark ? "dark" : "light"} ${width}`, async ({
        page,
      }, info) => {
        await page.setViewportSize({ width, height: 900 });
        const { content } = await openSealedTool(
          page,
          english,
          dark,
          "observe.catalog.query",
          {
            metrics: [],
            complete: false,
            has_more: false,
            warnings: ["SUPPORTED_METRIC_OBSERVATION_CHECK_INCOMPLETE"],
          },
        );
        await expect(
          content.getByText(
            english
              ? "Observation checks for some metrics are incomplete; this does not establish that they have no data."
              : "部分指标的观测情况尚未查全，不能据此认定它们没有数据。",
            { exact: true },
          ),
        ).toBeVisible();
        await expect(
          content.getByText(
            english
              ? "Partial; continue querying or inspect unavailable sections"
              : "结果不完整，请继续查询或检查不可用节",
            { exact: true },
          ),
        ).toBeVisible();
        await expect(
          content.getByText("SUPPORTED_METRIC_OBSERVATION_CHECK_INCOMPLETE", {
            exact: true,
          }),
        ).toHaveCount(0);
        await expect(
          content.getByText(
            english ? "No data in this range" : "当前范围没有数据",
            { exact: true },
          ),
        ).toHaveCount(0);
        await expect(
          content.getByText(
            english ? "Complete for this request" : "本次结果完整",
            { exact: true },
          ),
        ).toHaveCount(0);
        expect(
          await content
            .locator("body")
            .evaluate((body) => body.scrollWidth <= body.clientWidth + 1),
        ).toBe(true);
        await page.screenshot({
          path: info.outputPath("catalog-completeness.png"),
        });
        writeFileSync(
          info.outputPath("receipt.json"),
          JSON.stringify({
            passed: true,
            english,
            dark,
            width,
            actual_production_template: true,
            controlled_partial_result: true,
            model_or_real_backend_claimed: false,
          }),
        );
      });
