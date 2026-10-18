import { expect, test, type Page } from "@playwright/test";

test.use({ viewport: { width: 1440, height: 900 } });

async function login(page: Page, english: boolean, theme: string) {
  await page.addInitScript(
    ({ english, theme }) => {
      localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
      localStorage.setItem("argus.theme", theme);
    },
    { english, theme },
  );
  await page.goto("/login");
  await page.getByLabel(english ? "Username" : "用户名").fill("root");
  await page.getByLabel(english ? "Password" : "密码").fill("123456");
  await page
    .getByRole("button", { name: english ? "Sign in" : "登录", exact: true })
    .click();
  await expect(page).not.toHaveURL(/\/login/);
}

for (const english of [false, true]) {
  for (const theme of ["light", "dark"]) {
    test(`chat composer focus, centered icons and avatar-free messages ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      await login(page, english, theme);
      const composer = page.locator(".argus-chat-composer");
      const area = composer.getByRole("textbox");
      const box = composer.locator(".argus-chat-composer__box");
      await area.fill("检查主机状态");
      await expect
        .poll(() =>
          area.evaluate((el) => {
            const style = getComputedStyle(el);
            return {
              outline: style.outlineStyle,
              shadow: style.boxShadow,
              border: style.borderTopWidth,
              radius: style.borderRadius,
            };
          }),
        )
        .toEqual({
          outline: "none",
          shadow: "none",
          border: "0px",
          radius: "0px",
        });
      await expect
        .poll(() => box.evaluate((el) => getComputedStyle(el).outlineStyle))
        .toBe("solid");

      for (const button of await composer
        .locator("button.button--icon-only")
        .all()) {
        await button.hover();
        const alignment = await button.evaluate((el) => {
          const icon = el.querySelector("svg")!.getBoundingClientRect();
          const rect = el.getBoundingClientRect();
          return {
            horizontal: Math.abs(
              icon.x + icon.width / 2 - rect.x - rect.width / 2,
            ),
            vertical: Math.abs(
              icon.y + icon.height / 2 - rect.y - rect.height / 2,
            ),
          };
        });
        expect(alignment.horizontal).toBeLessThanOrEqual(1);
        expect(alignment.vertical).toBeLessThanOrEqual(1);
      }

      // @ still opens the resource picker and returns focus to the composer.
      await composer
        .getByRole("button", { name: english ? "Add" : "添加", exact: true })
        .click();
      await page
        .getByRole("menuitem", {
          name: english ? /^Reference a resource/ : /^引用资源/,
        })
        .click();
      await expect(page.getByRole("listbox")).toBeVisible();
      await expect(area).toBeFocused();
      await area.press("Escape");
      await area.fill("检查主机状态");

      await page.screenshot({
        path: info.outputPath("composer-focused.png"),
        animations: "disabled",
      });
      await composer
        .getByRole("button", { name: english ? "Send" : "发送", exact: true })
        .click();
      await expect(area).toHaveValue("", { timeout: 20_000 });
      const user = page.getByTestId("chat-message-user");
      await expect(user).toHaveCount(1);
      await expect(user).toContainText("检查主机状态");
      await expect(user.locator(".argus-avatar")).toHaveCount(0);
      await expect(page.locator(".argus-chat-message__body:empty")).toHaveCount(
        0,
      );
      await page.reload();
      await expect(user).toHaveCount(1);
      await expect(user.locator(".argus-avatar")).toHaveCount(0);
      await expect(page.locator(".argus-chat-message__body:empty")).toHaveCount(
        0,
      );
      await page.screenshot({
        path: info.outputPath("chat-messages.png"),
        animations: "disabled",
      });
    });
  }
}

test("upload and file preview survive reload without adding empty user replies", async ({
  page,
}, info) => {
  await login(page, false, "light");
  await page.getByRole("button", { name: "添加", exact: true }).click();
  await expect(
    page.getByRole("menuitem", { name: "上传文件", exact: true }),
  ).toBeEnabled();
  await page.keyboard.press("Escape");
  await page.locator('input[type="file"]').setInputFiles({
    name: "business.csv",
    mimeType: "text/csv",
    buffer: Buffer.from("value\n10\n20\n"),
  });
  await expect(
    page.locator(".argus-chat-chip").filter({ hasText: "business.csv" }),
  ).toBeVisible();
  await page
    .getByRole("textbox", { name: "发送", exact: true })
    .fill("检查主机状态");
  await page.getByRole("button", { name: "发送", exact: true }).click();
  await expect(
    page.getByRole("textbox", { name: "发送", exact: true }),
  ).toHaveValue("", { timeout: 20_000 });
  await page.reload();
  await expect(page.getByTestId("chat-message-user")).toHaveCount(1);
  await expect(
    page.getByTestId("chat-message-user").locator(".argus-avatar"),
  ).toHaveCount(0);
  await expect(page.locator(".argus-chat-message__body:empty")).toHaveCount(0);
  await page.getByRole("button", { name: "置顶摘要", exact: true }).click();
  await page.locator('summary[title="/workspace/uploads"]').click();
  await page.locator('summary[title^="/workspace/uploads/"]').click();
  await page
    .getByRole("button", { name: "预览 business.csv", exact: true })
    .click();
  const preview = page.getByRole("dialog", { name: "business.csv" });
  await expect(preview).toContainText("value");
  await expect(preview).toContainText("20");
  await page.screenshot({
    path: info.outputPath("uploaded-file-preview.png"),
    animations: "disabled",
  });
});
