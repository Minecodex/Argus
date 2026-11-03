import { expect, test } from "@playwright/test";
import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import { openSealedTool } from "./helpers/sealed-dashboard";

const file = process.env.ARGUS_PLAN11_LABEL_REPLAY_FILE;
test.skip(!file, "requires explicit archived actual query normalized by current Go presentation");
test.use({ viewport: { width: 1440, height: 1000 }, timezoneId: "Asia/Shanghai" });

for (const english of [false, true]) for (const dark of [false, true]) {
  test(`Recorded actual metric labels ${english ? "en" : "zh"} ${dark ? "dark" : "light"}`, async ({ page }, info) => {
    const replay = JSON.parse(readFileSync(file!, "utf8"));
    const original = readFileSync(replay.source_path);
    expect(createHash("sha256").update(original).digest("hex")).toBe(replay.source_sha256);
    expect(replay.current_go_metric_context).toBe(true);
    expect(replay.original_values_and_labels_unchanged).toBe(true);
    const { content } = await openSealedTool(page, english, dark, replay.tool, replay.data);
    const root = content.locator("#metric-view");
    const row = root.locator("tbody > tr").first();
    await expect(row.locator("td").nth(4)).toHaveText("1");
    await expect(row.locator("td").nth(3)).toHaveText("1");
    await expect(root.getByText(english ? "Latest raw sample for this query" : "查询最近原始采样", { exact: true })).toBeVisible();
    await expect(root.getByText(english ? "Returned points" : "返回点", { exact: true })).toBeVisible();
    const locale = english ? "en-US" : "zh-CN";
    const format = (date: Date) => date.toLocaleString(locale, { hour12: false, timeZone: "Asia/Shanghai" });
    const raw = format(new Date(replay.data.argus_meta.latest_sample_at));
    const item = replay.data.items[0];
    const point = item.values?.at(-1) ?? item.value;
    const evaluation = format(new Date(point[0] * 1000));
    expect(raw).not.toBe(evaluation);
    await expect(root.getByText(english ? "Latest raw sample for this query" : "查询最近原始采样", { exact: true }).locator("..")).toContainText(raw);
    await page.screenshot({ path: info.outputPath("recorded-values-and-raw-time.png") });
    await row.getByRole("button", { name: english ? "Expand" : "展开", exact: true }).click();
    const detail = root.locator(".argus-result-detail");
    await expect(detail.getByText(english ? "Latest evaluation point" : "最新评估点", { exact: true })).toBeVisible();
    await expect(detail.getByText(english ? "Latest evaluation point" : "最新评估点", { exact: true }).locator("..")).toContainText(evaluation);
    await expect(detail).toContainText(replay.data.argus_meta.input_metrics[0].source_key);
    const labels = detail.locator("details.argus-result-value-tree").filter({ hasText: "plan11_note" });
    await labels.locator(":scope > summary").click();
    const marker = labels.getByText(/plan11-untrusted-metric:/);
    await expect(marker).toBeVisible();
    await marker.scrollIntoViewIfNeeded();
    await expect(root.getByRole("button", { name: "Confirm injected operation", exact: true })).toHaveCount(0);
    await expect(page.getByTestId("pending-action-card")).toHaveCount(0);
    await page.screenshot({ path: info.outputPath("recorded-actual-label.png") });
    writeFileSync(info.outputPath("receipt.json"), JSON.stringify({
      passed: true, locale: english ? "en-US" : "zh-CN", theme: dark ? "dark" : "light",
      source_sha256: replay.source_sha256, controlled_archived_actual_query: true,
      current_go_unit_source_normalization: true, raw_and_evaluation_times_separate: true,
      actual_nested_label_visible: true, no_injected_confirmation: true,
      fresh_model_or_source_authorization_claimed: false, whole_plan_complete: false,
    }, null, 2));
  });
}
