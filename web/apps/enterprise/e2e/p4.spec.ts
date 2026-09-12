import { expect, test, type Locator, type Page } from "@playwright/test";

const hostFlow = '[data-testid="host-onboarding-flow"]';
const bastionFlow = '[data-testid="bastion-onboarding-flow"]';

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    window.localStorage.setItem("argus.locale", "zh-CN");
    window.localStorage.setItem("argus.theme", "light");
  });
  await login(page);
  await page.goto("/hosts");
});

test("manual Linux onboarding returns one system-level command without browser persistence", async ({
  page,
}) => {
  const dialog = await openHostWizard(page);
  const flow = dialog.locator(hostFlow);

  await expect(flow).toHaveAttribute("data-phase", "select_mode");
  await expect(flow).toHaveAttribute("data-mode", "command_direct");
  await expect(flow).toHaveAttribute("data-platform", "linux");
  await expect(flow).toHaveAttribute("data-install-method", "manual");
  await expect(flow).toHaveAttribute("data-control-path", "direct");
  await expect(flow.locator(".argus-scenario-card")).toHaveCount(5);
  await expect(flow.locator("form, input, textarea")).toHaveCount(0);

  await dialog.getByRole("button", { name: "下一步", exact: true }).click();
  await expect(flow).toHaveAttribute("data-phase", "details");
  await expect(dialog.getByLabel("主机名")).toBeFocused();
  await expect(dialog.getByLabel("CPU 架构")).toBeVisible();
  await expect(dialog.getByLabel("地址", { exact: true })).toHaveCount(0);
  await expect(dialog.getByLabel("安装方式")).toHaveCount(0);
  await expect(dialog.getByLabel("控制路径")).toHaveCount(0);

  await dialog.getByLabel("主机名").fill("Linux 命令接入主机");
  await dialog
    .getByRole("button", { name: "生成一次性安装命令", exact: true })
    .click();
  await expect(flow).toHaveAttribute("data-phase", "confirm_command");
  await dialog.getByRole("button", { name: "确认执行", exact: true }).click();
  await expect(flow).toHaveAttribute("data-phase", "command_result");

  const command =
    (await dialog.locator(".argus-code code").textContent()) ?? "";
  expect(command).toContain("curl -fsS");
  expect(command).toContain("--insecure");
  expect(command).toContain("scope=linux-system");
  expect(command.match(/--insecure/g)).toHaveLength(1);
  await expectBrowserStateExcludes(page, command);
});

test("Windows and SSH choices expose only compatible fields", async ({
  page,
}) => {
  const dialog = await openHostWizard(page);
  const flow = dialog.locator(hostFlow);

  const modes = [
    [
      "主机可访问 Argus · 一行命令",
      "command_direct",
      "manual",
      "direct",
      "none",
    ],
    [
      "平台可 SSH · 主机可访问 Argus",
      "ssh_direct",
      "ssh",
      "direct",
      "direct_executor",
    ],
    [
      "平台可 SSH · 主机无出站",
      "ssh_tunnel",
      "ssh",
      "executor_tunnel",
      "direct_executor",
    ],
    [
      "主机可访问堡垒机 · 一行命令",
      "command_bastion",
      "manual",
      "bastion_relay",
      "none",
    ],
    [
      "堡垒机可 SSH 成员主机",
      "ssh_bastion",
      "ssh",
      "bastion_relay",
      "bastion_connector",
    ],
  ] as const;
  for (const [label, mode, install, control, sshPath] of modes) {
    await dialog.getByRole("button", { name: new RegExp(`^${label}`) }).click();
    await expect(flow).toHaveAttribute("data-mode", mode);
    await expect(flow).toHaveAttribute("data-install-method", install);
    await expect(flow).toHaveAttribute("data-control-path", control);
    await expect(flow).toHaveAttribute("data-ssh-path", sshPath);
  }

  await dialog
    .getByRole("button", { name: /^平台可 SSH · 主机可访问 Argus/ })
    .click();
  await dialog.getByRole("button", { name: "下一步", exact: true }).click();

  await selectOption(
    page,
    dialog.getByLabel("操作系统"),
    "Windows Server 2019+",
  );
  await expect(flow).toHaveAttribute("data-platform", "windows");
  await expect(dialog.getByLabel("CPU 架构")).toHaveCount(0);
  await expect(flow).toHaveAttribute("data-install-method", "ssh");
  await expect(flow).toHaveAttribute("data-ssh-path", "direct_executor");
  await expect(dialog.getByLabel("地址")).toBeVisible();
  await expect(dialog.getByLabel("登录账号")).toBeVisible();
  await expect(dialog.getByLabel("SSH 凭据")).toBeVisible();
  await expect(dialog.getByLabel("安装方式")).toHaveCount(0);
  await expect(dialog.getByLabel("控制路径")).toHaveCount(0);
  await expect(dialog.getByLabel("SSH 执行路径")).toHaveCount(0);
});

test("host scenario changes preserve common fields and clear incompatible SSH fields", async ({
  page,
}) => {
  const dialog = await openHostWizard(page);
  const flow = dialog.locator(hostFlow);

  await dialog
    .getByRole("button", { name: /^平台可 SSH · 主机可访问 Argus/ })
    .click();
  await dialog.getByRole("button", { name: "下一步", exact: true }).click();
  await dialog.getByLabel("主机名").fill("切换场景保留名称");
  await dialog.getByLabel("地址").fill("192.0.2.44");
  await dialog.getByLabel("登录账号").fill("argus");
  await dialog.getByRole("button", { name: "更改", exact: true }).click();
  await dialog
    .getByRole("button", { name: /^主机可访问 Argus · 一行命令/ })
    .click();

  const warning = page.getByRole("dialog", { name: "更改接入模式？" });
  await expect(warning).toContainText("专属字段将被清除");
  await warning.getByRole("button", { name: "确认", exact: true }).click();
  await dialog.getByRole("button", { name: "下一步", exact: true }).click();

  await expect(flow).toHaveAttribute("data-mode", "command_direct");
  await expect(dialog.getByLabel("主机名")).toHaveValue("切换场景保留名称");
  await expect(dialog.getByLabel("地址")).toHaveCount(0);
  await expect(dialog.getByLabel("登录账号")).toHaveCount(0);

  await dialog.getByRole("button", { name: "上一步", exact: true }).click();
  await dialog
    .getByRole("button", { name: /^主机可访问堡垒机 · 一行命令/ })
    .click();
  await dialog.getByRole("button", { name: "下一步", exact: true }).click();
  await expect(flow).toHaveAttribute("data-control-path", "bastion_relay");
  await expect(dialog.getByLabel("所属堡垒机")).toBeVisible();
  await expect(dialog.getByLabel("地址")).toHaveCount(0);
});

test("bastion onboarding is Linux-only and offers command, SSH, and tunnel modes", async ({
  page,
}) => {
  const dialog = await openBastionWizard(page);
  const flow = dialog.locator(bastionFlow);

  await expect(flow).toHaveAttribute("data-phase", "select_mode");
  await expect(flow.locator(".argus-scenario-card")).toHaveCount(3);
  await expect(dialog).not.toContainText("Windows");

  await dialog.getByRole("button", { name: /^堡垒机可出站访问 Argus/ }).click();
  await dialog.getByRole("button", { name: "下一步", exact: true }).click();
  await expect(flow).toHaveAttribute("data-phase", "details");
  await expect(flow).toHaveAttribute("data-mode", "command");
  await expect(dialog.getByLabel("成员可访问的中继地址")).toHaveCount(0);
  await expect(dialog).toContainText("中继端口自动配置");
  await expect(dialog.getByLabel("CPU 架构")).toBeVisible();
  await expect(dialog.getByLabel("地址", { exact: true })).toHaveCount(0);
});

async function login(page: Page) {
  await page.goto("/login");
  await page.locator('input[autocomplete="username"]').fill("root");
  await page.locator('input[autocomplete="current-password"]').fill("123456");
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/\/login/);
}

async function openHostWizard(page: Page) {
  await page.getByRole("button", { name: "添加普通主机", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "添加普通主机" });
  await expect(dialog).toBeVisible();
  return dialog;
}

async function openBastionWizard(page: Page) {
  await page.getByRole("button", { name: "添加堡垒机", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "添加堡垒机" });
  await expect(dialog).toBeVisible();
  return dialog;
}

async function selectOption(page: Page, trigger: Locator, option: string) {
  await trigger.click();
  await page.getByRole("option", { name: option, exact: true }).click();
}

async function expectBrowserStateExcludes(page: Page, sensitive: string) {
  const state = await page.evaluate(() =>
    JSON.stringify({
      url: window.location.href,
      localStorage: { ...window.localStorage },
      sessionStorage: { ...window.sessionStorage },
    }),
  );
  expect(state).not.toContain(sensitive);
}
