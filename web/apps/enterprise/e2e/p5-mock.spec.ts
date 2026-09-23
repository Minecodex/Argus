import { expect, test, type Page } from "@playwright/test";

async function login(page: Page, username = "root") {
  await page.addInitScript(() => localStorage.setItem("argus.locale", "zh-CN"));
  await page.goto("/login");
  await page.getByLabel("用户名").fill(username);
  await page.getByLabel("密码").fill("123456");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page).not.toHaveURL(/\/login/);
}

test("enterprise MCP grants, write-only Basic credentials and remembered conversation selection", async ({
  page,
}) => {
  await login(page);
  await page.goto("/settings/mcp");
  await page.getByRole("button", { name: "新增连接" }).click();
  const drawer = page.getByRole("dialog", { name: "新增连接" });
  await drawer.getByLabel("名称").fill("Business reports");
  await drawer
    .getByLabel("Remote Streamable HTTP 地址")
    .fill("https://mcp.example.test/tools");
  await drawer.getByLabel("认证方式").click();
  await page.getByRole("option", { name: "Basic", exact: true }).click();
  await drawer.getByLabel("用户名").fill("report-user");
  await drawer.getByLabel("密码").fill("p5-test-password");
  await drawer.getByRole("checkbox", { name: "企业超级管理员" }).check();
  await drawer.getByRole("button", { name: "保存", exact: true }).click();
  await expect(drawer).not.toBeVisible();
  const row = page.getByRole("row").filter({ hasText: "Business reports" });
  await row.getByRole("button", { name: "编辑连接" }).click();
  const edit = page.getByRole("dialog", { name: "编辑连接" });
  await expect(edit.getByLabel("密码")).toHaveValue("");
  await page.keyboard.press("Escape");
  await page.goto("/");
  await page.getByRole("textbox", { name: "发送" }).fill("检查主机状态");
  await page.getByRole("button", { name: "发送", exact: true }).click();
  await expect(page).toHaveURL(/c=/);
  await page.getByText(/会话 MCP 连接 ·/).click();
  const selected = page.getByRole("checkbox", { name: /Business reports/ });
  await expect(selected).not.toBeChecked();
  await selected.click();
  await expect(page.getByText("会话 MCP 连接 · 1")).toBeVisible();
  await page.reload();
  await page.getByText("会话 MCP 连接 · 1").click();
  await expect(selected).toBeChecked();
});

test("uploaded bytes survive reload and explicit Workspace deletion removes the file", async ({
  page,
}) => {
  await login(page);

  await expect(page.getByRole("button", { name: "上传文件" })).toBeEnabled();
  await page
    .locator('input[type="file"]')
    .setInputFiles({
      name: "business.csv",
      mimeType: "text/csv",
      buffer: Buffer.from("value\n10\n20\n"),
    });
  await expect(
    page.locator(".argus-chat-chip").filter({ hasText: "business.csv" }),
  ).toBeVisible();
  await page.reload();
  await page.getByText(/会话文件 ·/).click();
  const file = page.getByRole("link", { name: "business.csv", exact: true });
  await expect(file).toBeVisible();
  const content = await page.evaluate(
    async (href) => (await fetch(href)).text(),
    (await file.getAttribute("href"))!,
  );
  expect(content).toBe("value\n10\n20\n");
  await page
    .getByRole("button", { name: "删除 Workspace", exact: true })
    .click();
  const confirm = page.getByRole("dialog", { name: "删除 Workspace" });
  await confirm
    .getByRole("button", { name: "删除 Workspace", exact: true })
    .click();
  await expect(file).not.toBeVisible();
  await page.reload();
  await expect(page.getByText("会话文件 · 0")).toBeVisible();
});
