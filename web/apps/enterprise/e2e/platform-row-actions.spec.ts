import { expect, test } from "@playwright/test";
import { platformOrigin } from "./origins";
import { openActionMenu, selectRowAction } from "./helpers/action-menu";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    const tx = (zh: string, en: string) => (english ? en : zh);
    test(`Platform row menus ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      test.setTimeout(60_000);
      await page.addInitScript(
        ({ english, theme }) => {
          localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
          localStorage.setItem("argus.theme", theme);
        },
        { english, theme },
      );
      await page.goto(`${platformOrigin}/login?initialized=true&reset=1`);
      await page.locator('input[autocomplete="username"]').fill("admin");
      await page
        .locator('input[autocomplete="current-password"]')
        .fill("123456");
      await page.locator('form button[type="submit"]').click();
      const mfa = page.locator('input[autocomplete="one-time-code"]');
      await expect
        .poll(
          async () => (await mfa.isVisible()) || !/\/login/.test(page.url()),
        )
        .toBe(true);
      if (await mfa.isVisible()) {
        await mfa.fill("123456");
        await page.locator('form button[type="submit"]').click();
      }
      await expect(page).not.toHaveURL(/login/);
      await page.goto(`${platformOrigin}/admins`);
      let rows = page.locator("tbody > tr");
      await expect(rows.first()).toBeVisible();
      for (const row of await rows.all())
        await expect(row.locator("td").last().getByRole("button")).toHaveCount(
          1,
        );
      await openActionMenu(rows.first());
      await page.screenshot({
        path: info.outputPath("administrator-menu.png"),
        animations: "disabled",
      });
      await page.keyboard.press("Escape");
      await page.goto(`${platformOrigin}/sandbox`);
      for (const [zh, en] of [
        ["服务连接", "Connections"],
        ["镜像", "Images"],
        ["Sandbox Profile", "Sandbox Profiles"],
        ["企业配额", "Enterprise quotas"],
        ["活动会话", "Active sessions"],
      ]) {
        await page
          .getByRole("tab", { name: tx(zh!, en!), exact: true })
          .click();
        rows = page.locator("tbody > tr");
        await expect(rows.first()).toBeVisible();
        for (const row of await rows.all()) {
          const cell = row.locator("td").last();
          const menuButton = cell.getByRole("button");
          if ((await menuButton.count()) === 0) {
            await expect(cell).toHaveText("—");
            continue;
          }
          await expect(menuButton).toHaveCount(1);
          await expect(menuButton).toHaveAccessibleName(
            /更多操作|More actions/,
          );
          await expect(menuButton).toHaveText("");
        }
        await expect(page.locator("tbody").getByRole("switch")).toHaveCount(0);
        if (en === "Sandbox Profiles" || en === "Images") {
          const row = rows.first();
          await expect(
            row.getByText(tx("已启用", "Enabled"), { exact: true }),
          ).toBeVisible();
          await selectRowAction(row, tx("停用", "Disable"));
          await expect(
            row.getByText(tx("已停用", "Disabled"), { exact: true }),
          ).toBeVisible();
          await selectRowAction(row, tx("启用", "Enable"));
          await expect(
            row.getByText(tx("已启用", "Enabled"), { exact: true }),
          ).toBeVisible();
        }
        if (en === "Enterprise quotas") {
          await selectRowAction(rows.first(), tx("编辑配额", "Edit quota"));
          await expect(page.locator(".argus-drawer")).toBeVisible();
          await page.keyboard.press("Escape");
        }
      }
    });
  }
