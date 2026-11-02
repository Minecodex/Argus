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
  (binding) => binding.Tool === "observe.capabilities.get",
)!.Data;

for (const english of [false, true])
  for (const dark of [false, true])
    for (const width of [1280, 1440, 1920])
      test(`Collector anchors remain unresolved ${english ? "en" : "zh"} ${dark ? "dark" : "light"} ${width}`, async ({ page }, info) => {
        await page.setViewportSize({ width, height: 900 });
        const { content } = await openSealedTool(
          page,
          english,
          dark,
          "observe.capabilities.get",
          data,
        );
        await expect(
          content.getByText(
            english
              ? "This capability request did not read telemetry samples; it does not prove historical data is absent."
              : "本次能力查询未读取遥测样本；这不代表目标历史数据不存在。",
            { exact: true },
          ),
        ).toBeVisible();
        await content
          .getByRole("button", { name: english ? "Expand" : "展开", exact: true })
          .click();
        await content
          .locator("details>summary")
          .filter({ hasText: english ? "Collection state" : "采集状态" })
          .click();
        for (const fact of [
          english ? "Configured collector" : "配置采集器",
          english ? "Effective collector" : "生效采集器",
          "a1111111-1111-4111-8111-111111111111",
          "b2222222-2222-4222-8222-222222222222",
          english
            ? "Host or Cluster placement has not been verified by this result"
            : "此结果尚未核对主机或集群归属",
          english
            ? "This section lists collection instances only; SDK endpoints and other sources need separate checks."
            : "本节仅列采集实例；SDK 接入点和其他来源需另行核对。",
        ]) await expect(content.getByText(fact, { exact: true }).first()).toBeVisible();
        await content
          .getByText(english ? "Configured collector" : "配置采集器", { exact: true })
          .scrollIntoViewIfNeeded();
        await content
          .locator("details>summary")
          .filter({ hasText: english ? /^Sources$/ : /^采集来源$/ })
          .click();
        for (const fact of [
          english ? "Sources on this page" : "本页来源数",
          english ? "Version entries on this page" : "本页版本项数",
          "c3333333-3333-4333-8333-333333333333:1",
          "c3333333-3333-4333-8333-333333333333:2",
          "c3333333-3333-4333-8333-333333333333:3",
        ]) await expect(content.getByText(fact, { exact: true }).first()).toBeVisible();
        await expect(content.locator("body")).toContainText(
          english ? "This page: 1 source, 3 version entries." : "本页：1 个来源，3 个版本项。",
        );
        await content
          .getByText(english ? "Sources on this page" : "本页来源数", { exact: true })
          .scrollIntoViewIfNeeded();
        expect(
          await content.locator("body").evaluate(
            (body) => body.scrollWidth <= body.clientWidth + 1,
          ),
        ).toBe(true);
        await page.screenshot({ path: info.outputPath("collector-anchors.png") });
        writeFileSync(
          info.outputPath("receipt.json"),
          JSON.stringify({
            passed: true,
            english,
            dark,
            width,
            configured_and_effective_distinct: true,
            placement_unresolved_explicit: true,
            controlled_production_template_fixture: true,
            real_model_claimed: false,
          }),
        );
      });
