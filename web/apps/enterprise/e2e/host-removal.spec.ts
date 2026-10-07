import { expect, test, type Locator, type Page } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    window.localStorage.setItem("argus.locale", "zh-CN");
    window.localStorage.setItem("argus.theme", "light");
  });
  await login(page);
  await page.goto("/hosts");
});

test("command-installed host is uninstalled before its record can be deleted", async ({
  page,
}) => {
  await seedRegisteredHost(page);
  let tile = page
    .locator(".argus-host-tile")
    .filter({ hasText: "public-web-01" });
  await tile.getByRole("button", { name: "卸载", exact: true }).click();

  const dialog = page.getByRole("dialog", { name: "卸载 Argus 软件" });
  await expect(dialog.getByText("命令卸载", { exact: true })).toBeVisible();
  await dialog
    .getByRole("button", { name: "生成卸载预览", exact: true })
    .click();
  await dialog.getByRole("button", { name: "确认执行", exact: true }).click();

  const command =
    (await dialog.locator(".argus-code code").textContent()) ?? "";
  expect(command).toContain("mock POSIX removal command");
  expect(command).not.toContain("--insecure");
  await dialog.getByRole("button", { name: "完成", exact: true }).click();

  tile = page.locator(".argus-host-tile").filter({ hasText: "public-web-01" });
  await expect(tile.getByText("已卸载", { exact: true })).toBeVisible();
  await tile.getByRole("button", { name: "更多操作", exact: true }).click();
  await page.getByRole("menuitem", { name: "删除", exact: true }).click();
  await page
    .getByRole("dialog", { name: "删除主机" })
    .getByRole("button", { name: "确认删除", exact: true })
    .click();
  await page
    .getByRole("dialog", { name: "删除主机" })
    .getByRole("button", { name: "确认执行", exact: true })
    .click();
  await expect(
    page
      .getByRole("dialog", { name: "删除主机" })
      .getByText(/等待审批/)
      .first(),
  ).toBeVisible();
  await expect(
    page.locator(".argus-host-tile").filter({ hasText: "public-web-01" }),
  ).toBeVisible();
});

test("bastion removal is blocked by linked member resources", async ({
  page,
}) => {
  const scope = page
    .locator(".argus-scope-card")
    .filter({ hasText: "上海机房堡垒机-01" });
  await scope
    .locator(".argus-scope-card__head")
    .getByRole("button", { name: "卸载", exact: true })
    .click();

  const dialog = page.getByRole("dialog", { name: "卸载 Argus 软件" });
  await dialog
    .getByRole("button", { name: "生成卸载预览", exact: true })
    .click();
  await expect(
    dialog.getByText("堡垒机仍被使用", { exact: true }),
  ).toBeVisible();
  const member = dialog.getByRole("link", { name: /host-web-11/ });
  await expect(member).toHaveAttribute("href", "/hosts/host-web-11");
  await expect(
    dialog.getByRole("button", { name: "确认执行", exact: true }),
  ).toHaveCount(0);
});

test("command-mode bastion exposes a governed Connector takeover", async ({
  page,
}) => {
  const scope = page
    .locator(".argus-scope-card")
    .filter({ hasText: "上海机房堡垒机-01" });
  await scope
    .locator(".argus-scope-card__head")
    .getByRole("button", { name: "替换 Connector", exact: true })
    .click();

  const dialog = page.getByRole("dialog", { name: "Connector 替换" });
  await dialog
    .getByRole("button", { name: "替换 Connector", exact: true })
    .click();
  await dialog.getByRole("button", { name: "确认执行", exact: true }).click();
  await expect(dialog.getByText(/等待审批/).first()).toBeVisible();
});

async function login(page: Page) {
  await page.goto("/login");
  await page.locator('input[autocomplete="username"]').fill("root");
  await page.locator('input[autocomplete="current-password"]').fill("123456");
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/\/login/);
}

for (const scenario of [
  "linux",
  "windows",
  "executor-tunnel",
  "member",
  "bastion",
] as const) {
  test(`SSH removal reuses saved credentials for ${scenario}`, async ({
    page,
  }, testInfo) => {
    await seedSSHRemoval(page, scenario);
    const launcher =
      scenario === "bastion"
        ? page
            .locator(".argus-scope-card")
            .filter({ hasText: "上海机房堡垒机-01" })
            .locator(".argus-scope-card__head")
        : page.locator(".argus-host-tile").filter({ hasText: "public-web-01" });
    await launcher.getByRole("button", { name: "卸载", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "卸载 Argus 软件" });
    await expect(
      dialog.getByText("使用已保存的 SSH 连接", { exact: true }),
    ).toBeVisible();
    await expect(
      dialog.getByText(/账号：install-admin；凭据：prod-ssh-key/),
    ).toBeVisible();
    await expect(dialog.getByRole("textbox")).toHaveCount(0);
    if (scenario === "linux") {
      await dialog.screenshot({
        path: testInfo.outputPath("saved-connection-light.png"),
      });
    }
    await dialog
      .getByRole("button", { name: "生成卸载预览", exact: true })
      .click();
    await expect(
      dialog.getByText("使用已保存的 SSH 连接", { exact: true }),
    ).toHaveCount(0);
    await expect(dialog.getByText("卸载准备失败", { exact: true })).toHaveCount(
      0,
    );
    await expect(
      dialog
        .getByRole("button", { name: "确认执行", exact: true })
        .or(dialog.getByText("堡垒机仍被使用", { exact: true })),
    ).toBeVisible();
  });
}

for (const scenario of [
  "linux",
  "windows",
  "executor-tunnel",
  "member",
] as const) {
  test(`failed ${scenario} Host can delete its unfinished record`, async ({
    page,
  }) => {
    await seedNeverRegisteredHost(page, scenario);
    const tile = page
      .locator(".argus-host-tile")
      .filter({ hasText: "public-web-01" });
    await tile.getByRole("button", { name: "卸载", exact: true }).click();
    const dialog = page.getByRole("dialog", {
      name: "删除未完成的安装记录",
    });
    await expect(dialog.getByLabel("SSH 账号")).toHaveCount(0);
    await dialog.getByRole("button", { name: "删除记录", exact: true }).click();
    await expect(dialog.getByText(/目标机可能保留安装残留/)).toBeVisible();
    await dialog.getByRole("button", { name: "确认删除", exact: true }).click();
    await expect(tile).toHaveCount(0);
  });
}

test("failed Host can delete its unfinished record from detail", async ({
  page,
}) => {
  const hostId = await seedNeverRegisteredHost(page, "linux");
  await page.goto(`/hosts/${hostId}`);
  await page
    .locator("main")
    .getByRole("button", { name: "卸载", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "删除未完成的安装记录",
  });
  await dialog.getByRole("button", { name: "删除记录", exact: true }).click();
  await dialog.getByRole("button", { name: "确认删除", exact: true }).click();
  await expect(page.getByText("主机不存在或已被删除")).toBeVisible();
});

test("cancelling an unfinished Host deletion keeps the record", async ({
  page,
}) => {
  await seedNeverRegisteredHost(page, "linux");
  const tile = page
    .locator(".argus-host-tile")
    .filter({ hasText: "public-web-01" });
  await tile.getByRole("button", { name: "卸载", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "删除未完成的安装记录",
  });
  await dialog.getByRole("button", { name: "删除记录", exact: true }).click();
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  await expect(tile).toBeVisible();
  await expect(
    dialog.getByRole("button", { name: "删除记录", exact: true }),
  ).toBeVisible();
});

test("stale registered Host can delete after defaults rejects it", async ({
  page,
}) => {
  await seedStaleRegisteredHost(page);
  await page
    .locator(".argus-host-tile")
    .filter({ hasText: "public-web-01" })
    .getByRole("button", { name: "卸载", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByText("删除未完成的安装记录", { exact: true }).first(),
  ).toBeVisible();
  await expect(
    dialog.getByRole("button", { name: "重试加载", exact: true }),
  ).toHaveCount(0);
  await expect(dialog.getByRole("group", { name: "选择处理方式" })).toHaveCount(
    0,
  );
  await expect(dialog.getByLabel("SSH 账号")).toHaveCount(0);
  await expect(
    dialog.getByRole("button", { name: "生成卸载预览", exact: true }),
  ).toHaveCount(0);
  await expect(
    dialog.getByRole("button", { name: "删除记录", exact: true }),
  ).toBeVisible();
  await dialog.getByRole("button", { name: "删除记录", exact: true }).click();
  await dialog.getByRole("button", { name: "确认删除", exact: true }).click();
  await expect(
    page.locator(".argus-host-tile").filter({ hasText: "public-web-01" }),
  ).toHaveCount(0);
});

for (const [name, deleteLabel] of [
  ["P4 代装进行中", "取消安装并删除"],
  ["P4 隧道代装失败", "删除记录"],
  ["P4 命令已领取", "删除记录"],
] as const) {
  test(`unfinished Bastion ${name} can be deleted`, async ({ page }) => {
    const card = page.locator(".argus-scope-card").filter({ hasText: name });
    await card.getByRole("button", { name: deleteLabel, exact: true }).click();
    await expect(card.getByText(/目标机可能保留安装残留/)).toBeVisible();
    await card.getByRole("button", { name: "确认删除", exact: true }).click();
    await expect(card).toHaveCount(0);
  });
}

test("failed Bastion root can be deleted from Host detail", async ({
  page,
}) => {
  const hostId = await seedFailedBastionRoot(page);
  await page.goto(`/hosts/${hostId}`);
  await page
    .locator("main")
    .getByRole("button", { name: "卸载", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "删除未完成的安装记录",
  });
  await dialog.getByRole("button", { name: "删除记录", exact: true }).click();
  await dialog.getByRole("button", { name: "确认删除", exact: true }).click();
  await expect(page.getByText("主机不存在或已被删除")).toBeVisible();
});

test("saved SSH account can be changed and resets when reopening", async ({
  page,
}) => {
  await seedSSHRemoval(page, "linux");
  const tile = page
    .locator(".argus-host-tile")
    .filter({ hasText: "public-web-01" });
  await tile.getByRole("button", { name: "卸载", exact: true }).click();
  let dialog = page.getByRole("dialog", { name: "卸载 Argus 软件" });
  await dialog.getByRole("button", { name: "更换账号或凭据" }).click();
  await expect(dialog.getByLabel("SSH 账号")).toHaveValue("install-admin");
  await dialog.getByLabel("SSH 账号").fill("changed-admin");
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  await tile.getByRole("button", { name: "卸载", exact: true }).click();
  dialog = page.getByRole("dialog", { name: "卸载 Argus 软件" });
  await expect(
    dialog.getByText(/账号：install-admin；凭据：prod-ssh-key/),
  ).toBeVisible();
  await expect(dialog.getByText(/changed-admin/)).toHaveCount(0);
});

test("missing installation credential preserves account and requests replacement", async ({
  page,
}) => {
  await seedSSHRemoval(page, "linux", true);
  await page
    .locator(".argus-host-tile")
    .filter({ hasText: "public-web-01" })
    .getByRole("button", { name: "卸载", exact: true })
    .click();
  const dialog = page.getByRole("dialog", { name: "卸载 Argus 软件" });
  await expect(
    dialog.getByText("请确认 SSH 连接信息", { exact: true }),
  ).toBeVisible();
  await expect(dialog.getByLabel("SSH 账号")).toHaveValue("install-admin");
  await dialog
    .getByRole("button", { name: "生成卸载预览", exact: true })
    .click();
  await expect(
    dialog.getByText("请选择 SSH 凭据并填写账号。", { exact: true }),
  ).toBeVisible();
});

test("saved SSH connection is available in English dark mode", async ({
  page,
}, testInfo) => {
  await seedSSHRemoval(page, "windows");
  await page.addInitScript(() => {
    localStorage.setItem("argus.locale", "en-US");
    localStorage.setItem("argus.theme", "dark");
  });
  await page.reload();
  await page
    .locator(".argus-host-tile")
    .filter({ hasText: "public-web-01" })
    .getByRole("button", { name: "Uninstall", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByText("Use saved SSH connection", { exact: true }),
  ).toBeVisible();
  await expect(
    dialog.getByText(/Account: install-admin; credential: prod-ssh-key/),
  ).toBeVisible();
  await expect(
    dialog.getByRole("button", { name: "Change account or credential" }),
  ).toBeVisible();
  await dialog.screenshot({
    path: testInfo.outputPath("saved-connection-dark.png"),
  });
});

for (const locale of ["zh-CN", "en-US"] as const) {
  test(`offline SSH host offers record removal without credentials in ${locale}`, async ({
    page,
  }, testInfo) => {
    await seedSSHRemoval(page, "linux", true);
    await page.addInitScript((locale) => {
      localStorage.setItem("argus.locale", locale);
      localStorage.setItem(
        "argus.theme",
        locale === "zh-CN" ? "light" : "dark",
      );
    }, locale);
    await page.reload();
    const zh = locale === "zh-CN";
    const tile = page
      .locator(".argus-host-tile")
      .filter({ hasText: "public-web-01" });
    await tile
      .getByRole("button", { name: zh ? "卸载" : "Uninstall", exact: true })
      .click();
    let dialog = page.getByRole("dialog");
    const choices = dialog.getByRole("group", {
      name: zh ? "选择处理方式" : "Choose an action",
    });
    await expect(choices.getByRole("button")).toHaveCount(2);
    await expect(choices.getByRole("button").first()).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    await choices
      .getByRole("button", {
        name: zh ? /^仅从 Argus 移除/ : /^Remove from Argus only/,
      })
      .click();
    dialog = page.getByRole("dialog", {
      name: zh ? "仅从 Argus 移除" : "Remove from Argus only",
      exact: true,
    });
    await expect(
      dialog.getByLabel(zh ? "SSH 账号" : "SSH account"),
    ).toHaveCount(0);
    await expect(
      dialog.getByText(zh ? "SSH 自动卸载" : "Automatic SSH uninstall", {
        exact: true,
      }),
    ).toHaveCount(0);
    const preview = dialog.getByRole("button", {
      name: zh ? "生成移除预览" : "Generate removal preview",
    });
    await preview.click();
    await expect(
      dialog.getByText(
        zh ? "输入的资源名称不匹配。" : "The resource name does not match.",
      ),
    ).toBeVisible();
    await dialog
      .getByLabel(
        zh ? "输入 public-web-01 以确认" : "Type public-web-01 to confirm",
      )
      .fill("public-web-01");
    await dialog.screenshot({
      path: testInfo.outputPath(`removal-choices-${locale}.png`),
    });
    await expectFocusRingUnclipped(
      dialog.getByLabel(
        zh ? "输入 public-web-01 以确认" : "Type public-web-01 to confirm",
      ),
    );
    await preview.click();
    await expect(
      dialog.getByRole("button", {
        name: zh ? "确认执行" : "Confirm",
        exact: true,
      }),
    ).toBeVisible();
    await expect(
      dialog.getByText(zh ? "卸载准备失败" : "Uninstall preparation failed", {
        exact: true,
      }),
    ).toHaveCount(0);
    // A preview never deletes the machine; final execution stays governed by PendingAction.
    await expect(tile).toBeVisible();
  });
}

test("online SSH hosts explain why record-only removal is unavailable", async ({
  page,
}, testInfo) => {
  await seedSSHRemoval(page, "windows");
  await page
    .locator(".argus-host-tile")
    .filter({ hasText: "public-web-01" })
    .getByRole("button", { name: "卸载", exact: true })
    .click();
  const dialog = page.getByRole("dialog", { name: "卸载 Argus 软件" });
  await expect(
    dialog.getByRole("button", { name: /^仅从 Argus 移除/ }),
  ).toBeDisabled();
  await expect(dialog.getByText("仅离线或卸载异常时可用")).toBeVisible();
  await dialog.screenshot({
    path: testInfo.outputPath("removal-choices-online.png"),
  });
});

async function expectFocusRingUnclipped(input: Locator) {
  await expect(input).toBeFocused();
  const clippedBy = await input.evaluate((element) => {
    const style = getComputedStyle(element);
    const extent = Math.max(
      0,
      parseFloat(style.outlineWidth) + parseFloat(style.outlineOffset),
    );
    const rect = element.getBoundingClientRect();
    const elementScaleX = rect.width / element.offsetWidth;
    const elementScaleY = rect.height / element.offsetHeight;
    const clipped: string[] = [];
    for (
      let parent = element.parentElement;
      parent;
      parent = parent.parentElement
    ) {
      const parentStyle = getComputedStyle(parent);
      const bounds = parent.getBoundingClientRect();
      const scaleX = parent.offsetWidth ? bounds.width / parent.offsetWidth : 1;
      const scaleY = parent.offsetHeight
        ? bounds.height / parent.offsetHeight
        : 1;
      const left = bounds.left + parent.clientLeft * scaleX;
      const top = bounds.top + parent.clientTop * scaleY;
      if (
        parentStyle.overflowX !== "visible" &&
        (rect.left - extent * elementScaleX < left - 0.5 ||
          rect.right + extent * elementScaleX >
            left + parent.clientWidth * scaleX + 0.5)
      ) {
        clipped.push(`${parent.className}: horizontal focus ring`);
      }
      if (
        parentStyle.overflowY !== "visible" &&
        (rect.top - extent * elementScaleY < top - 0.5 ||
          rect.bottom + extent * elementScaleY >
            top + parent.clientHeight * scaleY + 0.5)
      ) {
        clipped.push(`${parent.className}: vertical focus ring`);
      }
    }
    return clipped;
  });
  expect(
    clippedBy,
    "The focused input must keep its complete outline visible",
  ).toEqual([]);
}

async function seedSSHRemoval(
  page: Page,
  scenario: "linux" | "windows" | "executor-tunnel" | "member" | "bastion",
  missing = false,
) {
  await page.evaluate(
    ({ scenario, missing }) => {
      const key = Object.keys(localStorage).find((key) =>
        key.startsWith("argus-mock:db-"),
      )!;
      const db = JSON.parse(localStorage.getItem(key)!);
      const scope = db.bastionScopes.find(
        (entry: { name: string }) => entry.name === "上海机房堡垒机-01",
      );
      const host =
        scenario === "bastion"
          ? db.hosts.find(
              (entry: { id: string }) => entry.id === scope.connectorHostId,
            )
          : db.hosts.find(
              (entry: { name: string }) => entry.name === "public-web-01",
            );
      Object.assign(host, {
        installMethod: "ssh",
        installSSHPath:
          scenario === "member" ? "bastion_connector" : "direct_executor",
        installUsername: "install-admin",
        installCredentialId: missing
          ? "deleted-credential"
          : "cred-sec-ssh-prod",
        platform: scenario === "windows" ? "windows" : "linux",
        connectionStatus: scenario === "linux" ? "offline" : "online",
        onboardingState:
          scenario === "executor-tunnel" ? "install_failed" : "registered",
      });
      if (scenario === "executor-tunnel") host.controlPath = "executor_tunnel";
      if (scenario === "bastion") scope.onboardingMode = "direct_install";
      if (scenario === "member") {
        host.bastionScopeId = scope.id;
        host.controlPath = "bastion_relay";
      }
      if (scenario !== "bastion") {
        const template = db.connectors[0];
        host.connectorId = "conn-removal-host";
        db.connectors.push({
          ...template,
          id: host.connectorId,
          enterpriseId: host.enterpriseId,
          role: "host",
          name: host.name,
          hostId: host.id,
          bastionScopeId: host.bastionScopeId ?? "",
          capabilities: ["command", "artifact_tunnel", "remote_session"],
          managedHostCount: 0,
        });
      }
      localStorage.setItem(key, JSON.stringify(db));
    },
    { scenario, missing },
  );
  await page.reload();
}

async function seedRegisteredHost(page: Page) {
  await page.evaluate(() => {
    const key = Object.keys(localStorage).find((key) =>
      key.startsWith("argus-mock:db-"),
    )!;
    const db = JSON.parse(localStorage.getItem(key)!);
    const host = db.hosts.find(
      (entry: { name: string }) => entry.name === "public-web-01",
    );
    const template = db.connectors[0];
    host.connectorId = "conn-removal-host";
    db.connectors.push({
      ...template,
      id: host.connectorId,
      enterpriseId: host.enterpriseId,
      role: "host",
      name: host.name,
      hostId: host.id,
      bastionScopeId: "",
      capabilities: ["command", "artifact_tunnel", "remote_session"],
      managedHostCount: 0,
    });
    localStorage.setItem(key, JSON.stringify(db));
  });
  await page.reload();
}

async function seedNeverRegisteredHost(
  page: Page,
  scenario: "linux" | "windows" | "executor-tunnel" | "member",
) {
  const hostId = await page.evaluate((scenario) => {
    const key = Object.keys(localStorage).find((key) =>
      key.startsWith("argus-mock:db-"),
    )!;
    const db = JSON.parse(localStorage.getItem(key)!);
    const host = db.hosts.find(
      (entry: { name: string }) => entry.name === "public-web-01",
    );
    const scope = db.bastionScopes.find(
      (entry: { id: string }) => entry.id === "scope-sh",
    );
    Object.assign(host, {
      connectorId: undefined,
      removalOperationId: undefined,
      onboardingState: "install_failed",
      connectionStatus: "onboarding",
      platform: scenario === "windows" ? "windows" : "linux",
      controlPath:
        scenario === "executor-tunnel"
          ? "executor_tunnel"
          : scenario === "member"
            ? "bastion_relay"
            : "direct",
      bastionScopeId: scenario === "member" ? scope.id : undefined,
      installMethod: "ssh",
      installSSHPath:
        scenario === "member" ? "bastion_connector" : "direct_executor",
    });
    localStorage.setItem(key, JSON.stringify(db));
    return host.id as string;
  }, scenario);
  await page.reload();
  return hostId;
}

async function seedStaleRegisteredHost(page: Page) {
  await page.evaluate(() => {
    const key = Object.keys(localStorage).find((key) =>
      key.startsWith("argus-mock:db-"),
    )!;
    const db = JSON.parse(localStorage.getItem(key)!);
    const host = db.hosts.find(
      (entry: { name: string }) => entry.name === "public-web-01",
    );
    Object.assign(host, {
      connectorId: "stale-connector",
      installMethod: "ssh",
      installSSHPath: "direct_executor",
      installUsername: "install-admin",
      installCredentialId: "cred-sec-ssh-prod",
    });
    localStorage.setItem(key, JSON.stringify(db));
  });
  await page.reload();
}

async function seedFailedBastionRoot(page: Page) {
  const hostId = await page.evaluate(() => {
    const key = Object.keys(localStorage).find((key) =>
      key.startsWith("argus-mock:db-"),
    )!;
    const db = JSON.parse(localStorage.getItem(key)!);
    const scope = db.bastionScopes.find(
      (entry: { id: string }) => entry.id === "scope-p4-failed",
    );
    const template = db.hosts.find(
      (entry: { name: string }) => entry.name === "public-web-01",
    );
    const hostId = "host-p4-failed-root";
    db.hosts.push({
      ...template,
      id: hostId,
      name: "P4 隧道代装失败根主机",
      role: "bastion",
      bastionScopeId: scope.id,
      connectorId: undefined,
      onboardingState: "install_failed",
      onboardingOperationId: scope.onboardingOperationId,
      connectionStatus: "onboarding",
    });
    scope.connectorHostId = hostId;
    localStorage.setItem(key, JSON.stringify(db));
    return hostId;
  });
  await page.reload();
  return hostId;
}
