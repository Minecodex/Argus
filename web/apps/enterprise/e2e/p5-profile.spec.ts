import { expect, test } from "@playwright/test";
import { platformOrigin } from "./origins";

for (const locale of ["zh-CN", "en-US"] as const) {
  for (const theme of ["light", "dark"] as const) {
    test(`offline profile creation persists supported settings in ${locale} ${theme}`, async ({
      page,
    }) => {
      await page.addInitScript(
        ({ locale, theme }) => {
          localStorage.setItem("argus.locale", locale);
          localStorage.setItem("argus.theme", theme);
        },
        { locale, theme },
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
      await expect(page).not.toHaveURL(/\/login/);
      await page.goto(`${platformOrigin}/sandbox`);
      const zh = locale === "zh-CN";
      await page
        .getByRole("tab", {
          name: zh ? "Sandbox Profile" : "Sandbox Profiles",
          exact: true,
        })
        .click();
      await page
        .getByRole("button", {
          name: zh ? "新建 Profile" : "New profile",
          exact: true,
        })
        .click();
      const drawer = page.getByRole("dialog");
      await drawer
        .getByLabel(zh ? "名称" : "Name")
        .fill("P5 offline profile");
      await drawer.getByRole("combobox").click();
      await page.getByRole("option").first().click();
      await drawer.getByLabel(zh ? "CPU（核）" : "CPU (cores)").fill("2");
      await drawer.getByLabel(zh ? "内存（MB）" : "Memory (MB)").fill("1536");
      await drawer
        .getByLabel(zh ? "实例有效期（秒）" : "Instance lifetime (seconds)")
        .fill("240");
      await expect(drawer.getByRole("switch")).toHaveCount(0);
      await expect(drawer).not.toContainText("sandbox.profiles.");
      await drawer
        .getByRole("button", { name: zh ? "保存" : "Save", exact: true })
        .click();
      await expect(drawer).not.toBeVisible();
      await page.reload();
      await page
        .getByRole("tab", {
          name: zh ? "Sandbox Profile" : "Sandbox Profiles",
          exact: true,
        })
        .click();
      const row = page
        .getByRole("row")
        .filter({ hasText: "P5 offline profile" });
      await expect(row).toContainText("2 CPU / 1536 MiB");
      await expect(row).toContainText("240");
      await expect(row).toContainText(
        zh ? "离线业务分析" : "Offline business analysis",
      );
      await row.getByRole("switch").click();
      await expect(row.getByRole("switch")).not.toBeChecked();
    });
  }
}
