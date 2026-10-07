import { test, expect } from "@playwright/test";
import { createMfaLogin } from "./helpers/mfa-login";
import { selectTrigger } from "./helpers/select";
import {
  assertEditorLayout,
  assertFilterAlignment,
  assertInboxContents,
  assertNoSeriousAxe,
  assertViewport,
} from "./helpers/portal-layout";
const login = createMfaLogin("enterprise");
const enabled = process.env.ARGUS_PLANV2_E2E === "1";
for (const scenario of [
  { english: false, theme: "light", viewport: { width: 1280, height: 800 } },
  { english: false, theme: "dark", viewport: { width: 1440, height: 900 } },
  { english: true, theme: "light", viewport: { width: 1920, height: 1080 } },
  { english: true, theme: "dark", viewport: { width: 1280, height: 800 } },
])
  test.describe(`PlanV2 layout review real ${scenario.english ? "en" : "zh"} ${scenario.theme}`, () => {
    test.skip(!enabled, "Real PlanV2 deployment is required");
    test.use({ viewport: scenario.viewport });
    test.setTimeout(180000);
    test("Demo layout, real result view and populated approvals", async ({
      page,
    }, info) => {
      const tx = (zh: string, en: string) => (scenario.english ? en : zh);
      await page.addInitScript(({ english, theme }) => {
        localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
        localStorage.setItem("argus.theme", theme);
      }, scenario);
      await login(
        page,
        "/login",
        process.env.ARGUS_PLANV2_USERNAME!,
        process.env.ARGUS_PLANV2_PASSWORD!,
      );
      await page.goto(`/dashboards/${process.env.ARGUS_PLANV2_GALLERY_ID}`);
      const draftResponse = page.waitForResponse(
        (response) =>
          response.request().method() === "GET" &&
          /\/dashboard-drafts\/[^/]+$/.test(new URL(response.url()).pathname) &&
          response.status() === 200,
      );
      await page
        .getByRole("button", {
          name: tx("编辑", "Edit"),
          exact: true,
        })
        .click();
      const draft = await (await draftResponse).json();
      await page
        .getByRole("button", {
          name: tx("添加统计图", "Add panel"),
          exact: true,
        })
        .first()
        .click();
      await page
        .getByRole("button", {
          name: tx("自定义统计图", "Custom panel"),
          exact: true,
        })
        .click();
      await expect(page).toHaveURL(/\/panels\//);
      const panel = { id: new URL(page.url()).pathname.split("/").at(-1)! };
      await selectTrigger(page, tx("来源类型", "Source type")).click();
      await page
        .getByRole("option", { name: "OpenTelemetry", exact: true })
        .click();
      await page
        .getByRole("combobox", { name: tx("指标", "Metric"), exact: true })
        .fill("argus_m7_e2e_gauge_planv2");
      await page
        .getByRole("button", {
          name: tx("运行查询", "Run query"),
          exact: true,
        })
        .focus();
      await expect(
        page.locator(".argus-panel-editor__query-fields").getByText(/^gauge ·/),
      ).toBeVisible();
      await expect(
        page.getByText(tx("草稿已保存", "Draft saved"), { exact: false }),
      ).toBeVisible();
      await assertEditorLayout(page, scenario.english);
      const result = page.waitForResponse(
        (r) =>
          r.url().endsWith(`/dashboard-drafts/${draft.id}/sample`) &&
          r.status() === 200,
        { timeout: 30000 },
      );
      await page
        .getByRole("button", { name: tx("运行查询", "Run query"), exact: true })
        .click();
      const sampled = await (await result).json();
      expect(sampled.validation.valid).toBe(true);
      expect(
        sampled.sample.execution.panels.map((p: { id: string }) => p.id),
      ).toEqual([panel.id]);
      await expect(
        page.getByText(tx("最近运行结果", "Latest query result"), {
          exact: true,
        }),
      ).toBeVisible();
      let queryCount = 0;
      page.on("request", (r) => {
        if (r.url().endsWith("/sample")) queryCount++;
      });
      await page
        .getByRole("button", {
          name: tx("数据表格", "Data table"),
          exact: true,
        })
        .click();
      await expect(
        page.locator(".argus-panel-editor__visual").getByRole("table"),
      ).toBeVisible();
      await page
        .getByRole("button", {
          name: tx("返回图形", "Back to chart"),
          exact: true,
        })
        .click();
      await page
        .getByRole("tab", { name: tx("样式", "Style"), exact: true })
        .click();
      await page.getByLabel(tx("小数位数", "Decimal places")).fill("3");
      await expect(
        page.getByText(tx("草稿已保存", "Draft saved"), { exact: false }),
      ).toBeVisible();
      expect(queryCount).toBe(0);
      await assertNoSeriousAxe(page);
      await page.screenshot({
        path: info.outputPath("editor-real.png"),
        animations: "disabled",
      });
      await page.locator(".argus-page-content").evaluate((el) => {
        el.scrollTop = el.scrollHeight;
      });
      await assertViewport(
        page.getByRole("button", { name: tx("完成", "Done"), exact: true }),
        page,
      );
      await page
        .getByRole("button", { name: tx("完成", "Done"), exact: true })
        .click();
      await page
        .getByRole("button", {
          name: tx("预览并发布", "Preview and publish"),
          exact: true,
        })
        .click();
      await page
        .getByRole("dialog")
        .getByRole("button", { name: tx("取消", "Cancel"), exact: true })
        .click();
      await page.goto("/approvals?scope=created");
      await assertInboxContents(page);
      await assertNoSeriousAxe(page);
      await page.screenshot({
        path: info.outputPath("approvals-real.png"),
        animations: "disabled",
      });
      await page.goto("/hosts");
      await assertFilterAlignment(page);
      await page.screenshot({
        path: info.outputPath("hosts-real.png"),
        animations: "disabled",
      });
    });
  });
