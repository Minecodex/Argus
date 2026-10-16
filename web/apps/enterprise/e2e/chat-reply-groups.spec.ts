import { expect, test } from "@playwright/test";

for (const english of [false, true]) {
  test(`chat replies split only at user input ${english ? "English dark" : "Chinese light"}`, async ({
    page,
  }) => {
    await page.addInitScript((english) => {
      if (window.self !== window.top) return;
      localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
      localStorage.setItem("argus.theme", english ? "dark" : "light");
    }, english);
    await page.goto("/login");
    await page.getByLabel(english ? "Username" : "用户名").fill("root");
    await page.getByLabel(english ? "Password" : "密码").fill("123456");
    await page
      .getByRole("button", { name: english ? "Sign in" : "登录", exact: true })
      .click();
    await expect(page).not.toHaveURL(/\/login/);
    await page.goto("/");
    const composer = page.locator(".argus-chat-composer"),
      input = composer.getByRole("textbox");
    await input.fill(english ? "Check host health" : "查询主机状态");
    await composer
      .getByRole("button", { name: english ? "Send" : "发送", exact: true })
      .click();
    await expect(page).toHaveURL(/c=/);
    await expect(input).toHaveValue("", { timeout: 20000 });
    await expect(page.getByTestId("chat-assistant-heading")).toHaveCount(1);
    await expect(page.locator(".argus-conversation-item svg")).toHaveCount(0);
    await input.fill(english ? "Check host status again" : "再查询主机状态");
    await composer
      .getByRole("button", { name: english ? "Send" : "发送", exact: true })
      .click();
    await expect(input).toHaveValue("", { timeout: 20000 });
    await expect(page.getByTestId("chat-message-user")).toHaveCount(2);
    await expect(page.getByTestId("chat-assistant-heading")).toHaveCount(2);
    const turns = await page
      .locator(".argus-chat-stream__inner > [data-testid]")
      .evaluateAll((items) =>
        items.map((item) => item.getAttribute("data-testid")),
      );
    expect(turns).toEqual([
      "chat-message-user",
      "chat-assistant-reply",
      "chat-message-user",
      "chat-assistant-reply",
    ]);
    await page.reload();
    await expect(page.getByTestId("chat-assistant-heading")).toHaveCount(2);
    await expect(page.locator(".argus-conversation-item svg")).toHaveCount(0);
  });
}
