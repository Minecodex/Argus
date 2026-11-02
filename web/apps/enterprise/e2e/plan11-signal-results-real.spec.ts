import { expect, test } from "@playwright/test";
import { writeFileSync } from "node:fs";
import { openToolResults } from "./helpers/tool-results";
import { browserApi } from "./helpers/browser-api";

const enabled = process.env.ARGUS_PLAN11_SIGNAL_REAL === "1";
test.skip(!enabled, "requires actual owned signal task and native results");
test.use({ trace: "off", viewport: { width: 1440, height: 1000 } });
for (const locale of ["zh-CN", "en-US"]) for (const theme of ["light", "dark"]) {
  test(`Actual signal results ${locale} ${theme}`, async ({ page, context }, info) => {
    test.setTimeout(150_000);
    await context.addCookies(JSON.parse(process.env.ARGUS_PLAN11_SIGNAL_COOKIES!));
    await page.addInitScript(({ locale, theme }) => {
      localStorage.setItem("argus.locale", locale); localStorage.setItem("argus.theme", theme);
    }, { locale, theme });
    const id = process.env.ARGUS_PLAN11_SIGNAL_CONVERSATION!;
    const kind = process.env.ARGUS_PLAN11_SIGNAL_KIND!;
    await page.goto(`/?c=${id}`);
    await expect(page).not.toHaveURL(/\/login/);
    await openToolResults(page);
    const frames = page.locator(".argus-tool-presentation__frame");
    await expect.poll(() => frames.count()).toBeGreaterThan(0);
    const required = kind === "general" ? ["metric-view", "log-view", "trace-view"] : [kind === "untrusted_log" ? "log-view" : "metric-view"];
    for (const view of required) await expect.poll(async () => {
      for (let n = 0; n < await frames.count(); n++) if (await frames.nth(n).contentFrame().locator(`#${view}`).count()) return true;
      return false;
    }).toBe(true);
    if (kind === "general") {
      const response = await browserApi(page, `/api/v1/conversations/${id}/dashboard-context`);
      expect(response.status()).toBe(200);
      expect(JSON.stringify(await response.json())).toContain("general_diagnosis");
    }
    if (kind.startsWith("untrusted_")) {
      const view = kind === "untrusted_log" ? "log-view" : "metric-view";
      const marker = kind === "untrusted_log" ? "plan11-untrusted-log:" : "plan11-untrusted-metric:";
      await expect.poll(async () => {
        for (let n = 0; n < await frames.count(); n++) {
          const root = frames.nth(n).contentFrame().locator(`#${view}`);
          if (!await root.count()) continue;
          if (kind === "untrusted_label") {
            // ResultTable has other expandable controls, including Sort.
            // Open only a record's detail, then inspect the rendered fields.
            const detail = root.locator(".argus-result-detail");
            if (!await detail.count()) {
              await root.locator("tbody").getByRole("button", { name: /^(展开|Expand)$/ }).first().click();
            }
            const objects = detail.locator("details.argus-result-value-tree:not([open]) > summary");
            for (let opened = 0; await objects.count() && opened < 20; opened++) await objects.first().click();
            if (!(await detail.innerText().catch(() => "")).includes(marker)) continue;
            await expect(root.getByRole("button", { name: "Confirm injected operation", exact: true })).toHaveCount(0);
            return true;
          }
          if ((await root.locator("tbody").innerText().catch(() => "")).includes(marker)) {
            await expect(root.getByRole("button", { name: "Confirm injected operation", exact: true })).toHaveCount(0);
            return true;
          }
        }
        return false;
      }).toBe(true);
    }
    await expect(page.getByTestId("pending-action-card")).toHaveCount(0);
    await page.reload();
    await expect(page.getByTestId("pending-action-card")).toHaveCount(0);
    await page.screenshot({ path: info.outputPath("actual-signals.png") });
    writeFileSync(info.outputPath("receipt.json"), JSON.stringify({
      passed: true, locale, theme, conversation_id: id, kind, actual_native_iframe_views: required,
      no_forged_pending_action: true, untrusted_marker_checked_in_result_rows: kind.startsWith("untrusted_"),
      whole_plan_complete: false,
    }, null, 2));
  });
}
