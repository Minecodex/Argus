import { test, expect } from "@playwright/test";
import {
  assertEditorLayout,
  assertFilterAlignment,
  assertInboxContents,
  assertNoSeriousAxe,
  assertViewport,
} from "./helpers/portal-layout";
for (const viewport of [
  { width: 1280, height: 800 },
  { width: 1440, height: 900 },
  { width: 1920, height: 1080 },
])
  test.describe(`portal layout review ${viewport.width}`, () => {
    test.use({ viewport });
    test.setTimeout(120000);
    for (const english of [false, true])
      for (const theme of ["light", "dark"]) {
        const tx = (zh: string, en: string) => (english ? en : zh);
        test(`populated approvals, filters and Demo editor ${english ? "en" : "zh"} ${theme}`, async ({
          page,
        }, info) => {
          await page.addInitScript(
            ({ english, theme }) => {
              localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
              localStorage.setItem("argus.theme", theme);
            },
            { english, theme },
          );
          await page.goto("/login");
          await page.getByLabel(tx("用户名", "Username")).fill("root");
          await page.getByLabel(tx("密码", "Password")).fill("123456");
          await page
            .getByRole("button", { name: tx("登录", "Sign in"), exact: true })
            .click();
          await expect(page).not.toHaveURL(/login/);
          await page.goto("/approvals?scope=created");
          await assertInboxContents(page);
          await assertNoSeriousAxe(page);
          await page.screenshot({
            path: info.outputPath("approvals.png"),
            animations: "disabled",
          });
          await page
            .getByRole("radio", {
              name: tx("我发起的", "Created by me"),
              exact: true,
            })
            .focus();
          await page.keyboard.press("ArrowRight");
          await expect(
            page.getByRole("radio", {
              name: tx("已处理", "Handled"),
              exact: true,
            }),
          ).toBeChecked();
          await page.goto("/hosts");
          await expect(
            page.locator(".argus-resource-card").first(),
          ).toBeVisible();
          await assertFilterAlignment(page);
          await page
            .locator(".argus-filter-bar__select .argus-select__trigger")
            .first()
            .click();
          const list = page.getByRole("listbox");
          await expect(list).toBeVisible();
          await expect(list.locator('[data-slot="label"]').first()).toHaveCSS("font-size", "13px");
          await assertNoSeriousAxe(page);
          await page.screenshot({
            path: info.outputPath("hosts-options.png"),
            animations: "disabled",
          });
          await page.keyboard.press("Escape");
          await page.goto("/dashboards");
          await page
            .getByRole("button", {
              name: tx("新建仪表盘", "Create dashboard"),
              exact: true,
            })
            .click();
          await page
            .getByRole("textbox", { name: tx("名称", "Name"), exact: true })
            .fill("Layout acceptance");
          await page
            .getByRole("button", { name: tx("完成", "Done"), exact: true })
            .click();
          await page
            .getByRole("button", {
              name: tx("添加统计图", "Add panel"),
              exact: true,
            })
            .first()
            .click();
          await page
            .getByRole("button", {
              name: tx("CPU 使用率", "CPU utilization"),
              exact: true,
            })
            .click();
          await assertEditorLayout(page, english);
          await page.screenshot({
            path: info.outputPath("editor-unqueried.png"),
            animations: "disabled",
          });
          await page
            .getByRole("button", {
              name: tx("运行查询", "Run query"),
              exact: true,
            })
            .click();
          await expect(
            page.getByText(tx("最近运行结果", "Latest query result"), {
              exact: true,
            }),
          ).toBeVisible();
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
          await expect(
            page.getByText(tx("最近运行结果", "Latest query result"), {
              exact: true,
            }),
          ).toBeVisible();
          await assertNoSeriousAxe(page);
          await page.screenshot({
            path: info.outputPath("editor-result.png"),
            animations: "disabled",
          });
          await page
            .locator(".argus-panel-editor__advanced")
            .first()
            .locator(":scope > summary")
            .click();
          await page.locator(".argus-page-content").evaluate((el) => {
            el.scrollTop = el.scrollHeight;
          });
          await assertViewport(
            page.getByRole("button", { name: tx("完成", "Done"), exact: true }),
            page,
          );
        });
      }
  });
