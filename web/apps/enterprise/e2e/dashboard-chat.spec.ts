import { expect, test } from "@playwright/test";

for (const english of [false, true])
  test(
    "Chat dashboard selection persists and exits " +
      (english ? "English dark" : "Chinese light"),
    async ({ page }, testInfo) => {
      await page.addInitScript(
        ({ english }) => {
          localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
          localStorage.setItem("argus.theme", english ? "dark" : "light");
        },
        { english },
      );
      await page.goto("/login");
      await page.getByLabel(english ? "Username" : "用户名").fill("root");
      await page.getByLabel(english ? "Password" : "密码").fill("123456");
      await page
        .getByRole("button", {
          name: english ? "Sign in" : "登录",
          exact: true,
        })
        .click();
      await expect(page).not.toHaveURL(/\/login/);
      await page.goto("/dashboards");
      await page
        .getByRole("button", {
          name: english ? "Create dashboard" : "新建仪表盘",
          exact: true,
        })
        .click();
      const dialog = page.getByRole("dialog");
      await dialog
        .getByRole("textbox", { name: english ? "Name" : "名称", exact: true })
        .fill("Chat overview");
      await dialog
        .getByRole("button", { name: english ? "Done" : "完成", exact: true })
        .click();
      await page
        .getByRole("button", {
          name: english ? "Preview and publish" : "预览并发布",
        })
        .click();
      await page
        .getByRole("dialog")
        .getByRole("button", {
          name: english ? "Confirm publication" : "确认发布",
          exact: true,
        })
        .click();
      await expect(page).toHaveURL(/\/dashboards\//);
      await page.goto("/");
      const composer = page.locator(".argus-chat-composer");
      const text = composer.getByRole("textbox");
      await text.fill("@Chat");
      await page.getByRole("option", { name: "Chat overview" }).click();
      await expect(composer).toContainText("Chat overview");
      await text.fill(english ? "Check all panels" : "检查所有统计图");
      const send = composer.getByRole("button", {
        name: english ? "Send" : "发送",
        exact: true,
      });
      await send.click();
      await expect(page).toHaveURL(/c=/);
      await expect(text).toHaveValue("", { timeout: 20000 });
      await expect(page.getByTestId("chat-message-user").first()).toContainText(
        "Chat overview",
      );
      await page.reload();
      await expect(composer).toContainText("Chat overview");
      await text.fill(english ? "Continue" : "继续");
      await send.click();
      await expect(text).toHaveValue("", { timeout: 20000 });
      await expect(composer).toContainText("Chat overview");
      await page.screenshot({
        path: testInfo.outputPath("chat-dashboard-context.png"),
        fullPage: true,
      });
      await composer
        .getByRole("button", {
          name: english ? "Exit dashboard mode" : "退出仪表盘模式",
        })
        .click();
      await text.fill(english ? "General chat" : "普通会话");
      await send.click();
      await expect(text).toHaveValue("", { timeout: 20000 });
      await page.reload();
      await expect(composer).not.toContainText("Chat overview");
      await composer
        .getByRole("button", {
          name: english ? "Create dashboard" : "创建仪表盘",
          exact: true,
        })
        .click();
      await expect(text).toHaveValue(
        english ? "/create-dashboard " : "/创建仪表盘 ",
      );
      await text.fill("/创建仪表盘 New dashboard");
      await send.click();
      await expect(text).toHaveValue("", { timeout: 20000 });
      await page.reload();
      await expect(composer).toContainText(
        english ? "Create / edit dashboards" : "创建 / 编辑仪表盘",
      );
    },
  );
