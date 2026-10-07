import { test, expect, type Locator, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { selectTrigger } from "./helpers/select";
import { setChoice } from "./helpers/choice";
async function select(
  page: Page,
  scope: Page | Locator,
  label: string,
  option: string,
) {
  await selectTrigger(scope, label).click();
  await page.getByRole("option", { name: option, exact: true }).click();
}
for (const viewport of [
  { width: 1280, height: 800 },
  { width: 1440, height: 900 },
  { width: 1920, height: 1080 },
])
  for (const english of [false, true])
    for (const theme of ["light", "dark"])
      test.describe(`dashboard conditions ${viewport.width} ${english ? "en" : "zh"} ${theme}`, () => {
        test.use({ viewport, actionTimeout: 15000 });
        test.setTimeout(120000);
        test("grouped controls, separate defaults and filter workspace", async ({
          page,
        }, info) => {
          const tx = (zh: string, en: string) => (english ? en : zh);
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
          await page.goto("/dashboards");
          await page
            .getByRole("button", {
              name: tx("新建仪表盘", "Create dashboard"),
              exact: true,
            })
            .click();
          await page
            .getByRole("textbox", { name: tx("名称", "Name"), exact: true })
            .fill("Conditions review");
          await page
            .getByRole("button", { name: tx("完成", "Done"), exact: true })
            .click();
          await page
            .getByRole("button", {
              name: tx("仪表盘设置", "Dashboard settings"),
              exact: true,
            })
            .click();
          const settings = page.getByRole("dialog", {
            name: tx("仪表盘设置", "Dashboard settings"),
            exact: true,
          });
          await settings
            .getByRole("button", {
              name: tx("默认时间范围", "Default time range"),
              exact: true,
            })
            .click();
          const time = page.getByRole("dialog", {
            name: tx("默认时间范围", "Default time range"),
            exact: true,
          });
          await select(
            page,
            time,
            tx("常用范围", "Common ranges"),
            tx("最近 6 小时", "Last 6 hours"),
          );
          await time
            .getByRole("button", { name: tx("应用", "Apply"), exact: true })
            .click();
          await settings.getByRole("spinbutton").fill("10");
          await settings
            .getByRole("button", { name: tx("完成", "Done"), exact: true })
            .click();
          await page
            .getByRole("button", {
              name: tx("管理过滤项", "Manage filters"),
              exact: true,
            })
            .click();
          const filters = page.getByRole("dialog", {
            name: tx("管理过滤项", "Manage filters"),
            exact: true,
          });
          await expect(filters.getByRole("spinbutton")).toHaveCount(0);
          await expect(
            filters.getByRole("button", {
              name: tx("默认时间范围", "Default time range"),
              exact: true,
            }),
          ).toHaveCount(0);
          await filters
            .getByRole("button", {
              name: tx("添加变量", "Add variable"),
              exact: true,
            })
            .click();
          await filters
            .getByRole("textbox", {
              name: tx("变量名", "Variable name"),
              exact: true,
            })
            .fill("environment");
          await filters
            .getByRole("textbox", {
              name: tx("显示名称", "Display label"),
              exact: true,
            })
            .fill(tx("环境", "Environment"));
          await filters
            .getByRole("textbox", { name: tx("字段", "Field"), exact: true })
            .fill("resource_attributes.deployment.environment.name");
          await filters
            .getByRole("button", {
              name: tx("预览候选", "Preview candidates"),
              exact: true,
            })
            .click();
          await expect(
            filters.locator(".argus-variable-preview"),
          ).toContainText(tx("已读取", "Loaded"));
          await filters
            .getByRole("button", {
              name: tx("添加变量", "Add variable"),
              exact: true,
            })
            .click();
          await filters
            .getByRole("textbox", {
              name: tx("变量名", "Variable name"),
              exact: true,
            })
            .fill("service");
          await filters
            .getByRole("textbox", {
              name: tx("显示名称", "Display label"),
              exact: true,
            })
            .fill(tx("服务", "Service"));
          await filters
            .getByRole("textbox", { name: tx("字段", "Field"), exact: true })
            .fill("resource_attributes.service.name");
          await expect(
            filters.locator(".argus-variable-list__item"),
          ).toHaveCount(2);
          await expect(
            filters
              .locator(".argus-variable-editor")
              .getByRole("textbox", {
                name: tx("变量名", "Variable name"),
                exact: true,
              }),
          ).toHaveCount(1);
          await filters.locator(".argus-variable-list__item").first().click();
          await expect(
            filters.getByRole("textbox", {
              name: tx("变量名", "Variable name"),
              exact: true,
            }),
          ).toHaveValue("environment");
          await page.screenshot({
            path: info.outputPath("filter-workspace.png"),
            animations: "disabled",
          });
          const axe = await new AxeBuilder({ page }).analyze();
          expect(
            axe.violations.filter((v) =>
              ["serious", "critical"].includes(v.impact),
            ),
          ).toEqual([]);
          await filters
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
            .getByRole("button", {
              name: tx("确认发布", "Confirm publication"),
              exact: true,
            })
            .click();
          await expect(page).toHaveURL(/\/dashboards\//);
          const toolbar = page.getByRole("group", {
            name: tx("仪表盘查询条件", "Dashboard query conditions"),
            exact: true,
          });
          const range = toolbar.getByRole("button", {
            name: tx("时间范围", "Time range"),
            exact: true,
          });
          const resources = toolbar.getByRole("button", {
            name: tx("资源范围", "Resource scope"),
            exact: true,
          });
          await expect(range).toContainText(tx("最近 6 小时", "Last 6 hours"));
          const a = (await range.boundingBox())!,
            b = (await resources.boundingBox())!;
          expect(b.x - a.x - a.width).toBeGreaterThanOrEqual(7);
          expect(b.x - a.x - a.width).toBeLessThanOrEqual(9);
          const heights = await toolbar
            .locator(".argus-button, .argus-select__trigger")
            .evaluateAll((items) =>
              items.map((el) => el.getBoundingClientRect().height),
            );
          expect(heights.every((height) => height === 32)).toBe(true);
          await resources.click();
          const resourcePopup = page.getByRole("dialog", {
            name: tx("选择资源", "Choose resources"),
            exact: true,
          });
          await expect(resourcePopup).toContainText(
            tx("选择不会授予权限", "Selection does not grant access"),
          );
          await resourcePopup.press("Escape");
          await expect(resourcePopup).not.toBeVisible();
          await expect(resources).toBeFocused();
          await range.click();
          const picker = page.getByRole("dialog", {
            name: tx("时间范围", "Time range"),
            exact: true,
          });
          await select(
            page,
            picker,
            tx("常用范围", "Common ranges"),
            tx("最近 24 小时", "Last 24 hours"),
          );
          await picker
            .getByRole("button", { name: tx("应用", "Apply"), exact: true })
            .click();
          await select(
            page,
            toolbar,
            tx("刷新间隔", "Refresh interval"),
            tx("自动刷新关闭", "Auto refresh off"),
          );
          await page.screenshot({
            path: info.outputPath("viewing-controls.png"),
            animations: "disabled",
          });
          await page.reload();
          await expect(range).toContainText(tx("最近 6 小时", "Last 6 hours"));
          await expect(
            selectTrigger(toolbar, tx("刷新间隔", "Refresh interval")),
          ).toContainText(tx("每 10 秒", "Every 10 s"));
          await toolbar
            .getByRole("button", {
              name: tx("环境", "Environment"),
              exact: true,
            })
            .click();
          const variablePopup = page.getByRole("dialog", {
            name: tx("环境", "Environment"),
            exact: true,
          });
          await setChoice(
            variablePopup.getByRole("checkbox", {
              name: tx("全部", "All"),
              exact: true,
            }),
            true,
          );
          await variablePopup
            .getByRole("button", { name: tx("取消", "Cancel"), exact: true })
            .click();
        });
      });
