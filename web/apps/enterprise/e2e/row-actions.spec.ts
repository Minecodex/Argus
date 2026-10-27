import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { openActionMenu, selectRowAction } from "./helpers/action-menu";

async function executionState(page: Page) {
  return page.evaluate(() => {
    const key = Object.keys(localStorage).find((key) =>
      key.startsWith("argus-mock:db-"),
    )!;
    const db = JSON.parse(localStorage.getItem(key)!);
    return {
      actions: db.pendingActions.length,
      executions: db.executions.length,
      collector: db.collectors.find(
        (c: { id: string }) => c.id === "col-db-01",
      ),
    };
  });
}

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    const tx = (zh: string, en: string) => (english ? en : zh);
    test.describe(`Row actions ${english ? "en" : "zh"} ${theme}`, () => {
      test.setTimeout(60_000);
      test.use({ viewport: { width: 1440, height: 1000 } });
      test.beforeEach(async ({ page }) => {
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
        await page.evaluate(() => {
          const key = Object.keys(localStorage).find((key) =>
            key.startsWith("argus-mock:db-"),
          )!;
          const db = JSON.parse(localStorage.getItem(key)!);
          const collector = db.collectors.find(
            (c: { id: string }) => c.id === "col-db-01",
          );
          collector.route = {
            id: "route-row-actions",
            collector_id: collector.id,
            kind: "direct_argus",
            transport: "direct",
            status: "active",
            version: 1,
            created_at: collector.created_at,
            updated_at: collector.updated_at,
          };
          localStorage.setItem(key, JSON.stringify(db));
        });
      });

      test("collector card menu provides route test and preview while detail omits lifecycle actions", async ({
        page,
      }, info) => {
        await page.goto("/hosts/host-db-master-01#otlp-collector");
        const card = page.locator(".argus-card").filter({
          has: page.locator(".argus-card__title").filter({ hasText: /OTLP/ }),
        });
        await expect(card.locator(".argus-setting-card")).toHaveCount(6);
        await expect(page.locator(".argus-collector-controls")).toHaveCount(0);
        await expect(
          page.getByRole("heading", {
            name: tx("生命周期操作", "Lifecycle actions"),
          }),
        ).toHaveCount(0);
        await expect(
          page.getByRole("button", {
            name: /卸载|Uninstall|升级|Upgrade|修复|Repair/,
          }),
        ).toHaveCount(0);
        const before = await executionState(page);
        let menu = await openActionMenu(card);
        await expect(menu.getByRole("menuitem")).toHaveCount(2);
        await page.screenshot({
          path: info.outputPath("collector-route-menu.png"),
          animations: "disabled",
        });
        await menu
          .getByRole("menuitem", {
            name: tx("路由测试", "Test route"),
            exact: true,
          })
          .click();
        const dialog = page.getByRole("dialog", {
          name: tx("数据推送路由", "Push route"),
          exact: true,
        });
        await expect(
          dialog.getByText(tx("路由可达", "Route reachable"), { exact: true }),
        ).toBeVisible();
        await page.keyboard.press("Escape");
        expect(await executionState(page)).toEqual(before);
        menu = await openActionMenu(card);
        await menu
          .getByRole("menuitem", {
            name: tx("修改路由", "Change route"),
            exact: true,
          })
          .click();
        await dialog
          .getByRole("button", {
            name: tx("预览变更", "Preview changes"),
            exact: true,
          })
          .click();
        await expect(
          dialog.getByRole("button", {
            name: tx("确认执行", "Confirm"),
            exact: true,
          }),
        ).toBeVisible();
        await dialog
          .getByRole("button", { name: tx("取消", "Cancel"), exact: true })
          .click();
        await expect(dialog).not.toBeVisible();
        const after = await executionState(page);
        expect(after.executions).toBe(before.executions);
        expect(after.collector).toEqual(before.collector);
      });

      test("host lifecycle actions share one menu and keep execution behind confirmation", async ({
        page,
      }, info) => {
        await page.goto("/hosts");
        const row = page.getByRole("row").filter({
          has: page.getByRole("link", { name: "db-master-01", exact: true }),
        });
        await expect(row.getByRole("button")).toHaveCount(1);
        const before = await executionState(page);
        const trigger = row.getByRole("button", {
          name: /更多操作|More actions/,
        });
        await trigger.focus();
        await trigger.press("Enter");
        const menu = page.getByRole("menu");
        await expect(
          menu.getByRole("menuitem", {
            name: tx("卸载主机", "Uninstall host"),
            exact: true,
          }),
        ).toHaveClass(/is-danger/);
        for (const [zh, en] of [
          ["升级采集器", "Upgrade collector"],
          ["修复采集器", "Repair collector"],
          ["卸载采集器", "Uninstall collector"],
        ]) {
          await expect(
            menu.getByRole("menuitem", { name: tx(zh!, en!), exact: true }),
          ).toBeVisible();
        }
        await page.screenshot({
          path: info.outputPath("host-lifecycle-menu.png"),
          animations: "disabled",
        });
        const audit = await new AxeBuilder({ page }).analyze();
        expect(
          audit.violations.filter((v) =>
            ["serious", "critical"].includes(v.impact ?? ""),
          ),
        ).toEqual([]);
        await page.keyboard.press("Escape");
        await expect(trigger).toBeFocused();
        expect(await executionState(page)).toEqual(before);
        for (const [zh, en] of [
          ["升级采集器", "Upgrade collector"],
          ["修复采集器", "Repair collector"],
          ["卸载采集器", "Uninstall collector"],
        ]) {
          await selectRowAction(row, tx(zh!, en!));
          const dialog = page.getByRole("dialog", {
            name: tx(zh!, en!),
            exact: true,
          });
          await expect(
            dialog.getByRole("button", {
              name: tx("确认执行", "Confirm"),
              exact: true,
            }),
          ).toBeVisible();
          await dialog
            .getByRole("button", { name: tx("取消", "Cancel"), exact: true })
            .click();
          await expect(dialog).not.toBeVisible();
        }
        const after = await executionState(page);
        expect(after.executions).toBe(before.executions);
        expect(after.collector).toEqual(before.collector);
        await selectRowAction(row, tx("卸载主机", "Uninstall host"));
        await expect(
          page.getByRole("dialog", {
            name: tx("卸载 Argus 软件", "Uninstall Argus software"),
            exact: true,
          }),
        ).toBeVisible();
      });

      test("resource, settings, alert and session lists use an icon menu for row operations", async ({
        page,
      }) => {
        await page.goto("/dashboards");
        await page
          .getByRole("button", {
            name: tx("新建仪表盘", "Create dashboard"),
            exact: true,
          })
          .click();
        await page
          .getByRole("textbox", { name: tx("名称", "Name"), exact: true })
          .fill("Row actions dashboard");
        await page
          .getByRole("button", { name: tx("完成", "Done"), exact: true })
          .click();
        await expect(page).toHaveURL(/dashboard-drafts/);
        await page.goto("/settings/mcp");
        await page
          .getByRole("button", {
            name: tx("新增连接", "Add connection"),
            exact: true,
          })
          .click();
        const connection = page.getByRole("dialog", {
          name: tx("新增连接", "Add connection"),
          exact: true,
        });
        await connection
          .getByRole("textbox", { name: tx("名称", "Name"), exact: true })
          .fill("Row actions MCP");
        await connection
          .getByRole("textbox", { name: /Remote Streamable HTTP/ })
          .fill("https://mcp.example.test/tools");
        await connection
          .getByRole("button", { name: tx("保存", "Save"), exact: true })
          .click();
        await expect(connection).not.toBeVisible();
        for (const url of [
          "/hosts",
          "/kubernetes",
          "/dashboards",
          "/settings/org",
          "/settings/secrets",
          "/settings/mcp",
          "/alerts",
          "/remote-sessions",
        ]) {
          await page.goto(url);
          if (url === "/remote-sessions")
            await page
              .getByRole("tab", {
                name: tx("会话录像", "Recordings"),
                exact: true,
              })
              .click();
          if (url === "/dashboards")
            await page
              .getByRole("tab", {
                name: tx("我的草稿", "My drafts"),
                exact: true,
              })
              .click();
          const tables = page.getByRole("table").filter({
            has: page.getByRole("columnheader", {
              name: tx("操作", "Actions"),
              exact: true,
            }),
          });
          await expect(tables.first()).toBeVisible();
          const rows = tables.locator("tbody > tr");
          await expect(rows.first(), url).toBeVisible();
          for (const row of await rows.all()) {
            const cell = row.locator("td").last();
            await expect(cell.getByRole("button"), url).toHaveCount(1);
            await expect(cell.getByRole("button"), url).toHaveAccessibleName(
              /更多操作|More actions/,
            );
            await expect(cell.getByRole("button"), url).toHaveText("");
          }
        }
      });
    });
  }
