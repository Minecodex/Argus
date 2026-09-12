import { expect, test, type Page } from "@playwright/test";

const modes = [
  ["host", "主机可访问 Argus · 一行命令", "command_direct", false, false],
  ["host", "平台可 SSH · 主机可访问 Argus", "ssh_direct", true, false],
  ["host", "平台可 SSH · 主机无出站", "ssh_tunnel", true, false],
  ["host", "主机可访问堡垒机 · 一行命令", "command_bastion", false, true],
  ["host", "堡垒机可 SSH 成员主机", "ssh_bastion", true, true],
  ["bastion", "堡垒机可出站访问 Argus", "command", false, false],
  ["bastion", "平台代装（双向可达）", "direct_install", true, false],
  [
    "bastion",
    "平台代装 + 控制隧道（堡垒机无出站）",
    "direct_install_tunnel",
    true,
    false,
  ],
] as const;

async function login(page: Page, language = "zh-CN", theme = "light") {
  await page.addInitScript(
    ({ language, theme }) => {
      localStorage.setItem("argus.locale", language);
      localStorage.setItem("argus.theme", theme);
    },
    { language, theme },
  );
  await page.goto("/login");
  await page.locator('input[autocomplete="username"]').fill("root");
  await page.locator('input[autocomplete="current-password"]').fill("123456");
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/\/login/);
  await page.goto("/hosts");
  await expect(page).toHaveTitle(/Argus/i);
}

for (const [kind, label, mode, ssh, relay] of modes) {
  test(`${kind} ${mode}: duplicate name stays editable and corrected name reaches preview`, async ({
    page,
  }, info) => {
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    page.on("console", (message) => {
      if (message.type() === "error") errors.push(message.text());
    });
    await login(page, "zh-CN", ssh ? "dark" : "light");
    const title = kind === "host" ? "添加普通主机" : "添加堡垒机";
    await page.getByRole("button", { name: title, exact: true }).click();
    const dialog = page.getByRole("dialog", { name: title });
    await dialog
      .locator(".argus-scenario-card")
      .filter({ has: page.getByText(label, { exact: true }) })
      .click();
    await dialog.getByRole("button", { name: "下一步", exact: true }).click();
    const flow = dialog.getByTestId(`${kind}-onboarding-flow`);
    const input = dialog.getByRole("textbox", {
      name: kind === "host" ? "主机名" : "名称",
      exact: true,
    });
    if (relay) {
      await dialog.getByLabel("所属堡垒机").click();
      await page.getByRole("option").first().click();
    }
    if (ssh) {
      await dialog
        .getByRole("textbox", { name: "地址", exact: true })
        .fill("10.20.30.40");
      await dialog
        .getByRole("textbox", { name: "登录账号", exact: true })
        .fill("root");
      await dialog
        .getByLabel(kind === "host" ? "SSH 凭据" : "登录凭据或密钥")
        .click();
      await page.getByRole("option").first().click();
    }
    await input.fill("PUBLIC-WEB-01");
    await input.press("Tab");
    await expect(
      dialog.getByText("该名称已被未删除的资源使用，请更换名称后重试。"),
    ).toBeVisible();
    await expect(input).toHaveAttribute("aria-invalid", "true");
    const submit = dialog.locator('button[type="submit"]');
    await submit.click();
    await expect(submit).toBeEnabled();
    await expect(flow).toHaveAttribute("data-phase", "details");
    await expect(
      dialog.getByRole("button", { name: "确认执行", exact: true }),
    ).toHaveCount(0);
    await expect(page.locator("vite-error-overlay")).toHaveCount(0);
    await page.screenshot({ path: info.outputPath("duplicate-name.png") });
    await input.fill(`name-check-${mode}`);
    await expect(input).not.toHaveAttribute("aria-invalid", "true");
    await submit.click();
    await expect(flow).toHaveAttribute(
      "data-phase",
      ssh ? "verify" : "confirm_command",
    );
    await expect(
      dialog.getByRole("button", { name: "确认执行", exact: true }),
    ).toBeVisible();
    if (ssh) {
      await expect(
        dialog.getByText("目标主机访问平台回调: 通过", { exact: true }),
      ).toBeVisible();
    } else {
      await expect(
        dialog.getByText("连接测试结果", { exact: true }),
      ).toHaveCount(0);
    }
    expect(errors).toEqual([]);
  });
}

test("English dark desktop shows a readable name conflict and clears it on edit", async ({
  page,
}, info) => {
  await login(page, "en-US", "dark");
  await page.getByRole("button", { name: "Add Host", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Add Host", exact: true });
  await dialog.getByRole("button", { name: "Next", exact: true }).click();
  const input = dialog.getByRole("textbox", { name: "Host name", exact: true });
  await input.fill("public-web-01");
  await input.press("Tab");
  await expect(
    dialog.getByText(
      "This name is already used by a resource that has not been deleted. Choose another name.",
    ),
  ).toBeVisible();
  await page.screenshot({
    path: info.outputPath("english-dark-duplicate.png"),
  });
  await input.fill("another-name");
  await expect(input).not.toHaveAttribute("aria-invalid", "true");
});
