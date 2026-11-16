import { expect, test } from "@playwright/test";
import { readFileSync, writeFileSync } from "node:fs";
import { selectRowAction } from "./helpers/action-menu";

const enabled = process.env.ARGUS_PLAN9_B11C_REAL === "1";
const mapping = enabled
  ? JSON.parse(readFileSync(process.env.ARGUS_PLAN9_B11C_MAP!, "utf8"))
  : {};
test.skip(
  !enabled,
  "Requires the actually isolated B11c Bastion/member deployment",
);
test.use({
  trace: "off",
  video: "off",
  ignoreHTTPSErrors: false,
  reducedMotion: "reduce",
});
const scenarios = [
  { key: "member", protocol: "shell", locale: "zh-CN", theme: "light" },
  { key: "member", protocol: "ssh", locale: "zh-CN", theme: "dark" },
  { key: "bastion", protocol: "shell", locale: "en-US", theme: "light" },
  { key: "bastion", protocol: "ssh", locale: "en-US", theme: "dark" },
];

test("B11c access revocation closes the actual PTY and restores the review grant", async ({
  page,
  browser,
}, info) => {
  test.setTimeout(150000);
  const login = async (target: typeof page) => {
    await target.addInitScript(() => {
      localStorage.setItem("argus.locale", "zh-CN");
      localStorage.setItem("argus.theme", "light");
    });
    await target.goto("/login");
    await target.locator('input[autocomplete="username"]').fill("kakj");
    await target
      .locator('input[autocomplete="current-password"]')
      .fill(process.env.ARGUS_PLAN9_PASSWORD!);
    await target.locator('form button[type="submit"]').click();
    await expect(target).not.toHaveURL(/\/login/);
    return target.evaluate(
      async () =>
        (await (await fetch("/api/v1/enterprise/auth/session")).json())
          .csrf_token as string,
    );
  };
  const csrf = await login(page);
  const frames: { type: string; session_id?: string }[] = [];
  page.on("websocket", (socket) =>
    socket.on("framereceived", (event) => {
      try {
        const value = JSON.parse(String(event.payload));
        frames.push({ type: value.type, session_id: value.session_id });
      } catch {
        /* Other traffic. */
      }
    }),
  );
  const state = mapping.hosts.bastion;
  await page.goto(`/hosts/${state.host_id}`);
  await page
    .getByRole("tab", { name: /终端与会话|Terminal & Sessions/ })
    .click();
  await page.getByLabel(/^(协议|Protocol)/).click();
  await page.getByRole("option", { name: "Shell / PTY", exact: true }).click();
  await page.getByLabel(/^(登录账号|Account)/).click();
  await page.getByRole("option", { name: "root", exact: true }).click();
  await page
    .getByLabel(/事由|Reason/)
    .fill("B11c actual grant revocation proof");
  await page.getByRole("button", { name: /^(建立会话|Open session)$/ }).click();
  await expect
    .poll(() => frames.find((frame) => frame.type === "server_ready"), {
      timeout: 45000,
    })
    .toBeTruthy();
  const id = frames.find((frame) => frame.type === "server_ready")!.session_id!;
  const adminContext = await browser.newContext({
    baseURL: process.env.ARGUS_E2E_ENTERPRISE_ORIGIN,
    ignoreHTTPSErrors: false,
  });
  const admin = await adminContext.newPage();
  let disabled = false;
  let adminCsrf = "";
  const grantPath = `/api/v1/enterprise/remote-access-grants/${mapping.remote.grant_id}`;
  try {
    const result = await page.evaluate(
      async ({ path, csrf, id }) => {
        const current = await (await fetch(path)).json();
        const response = await fetch(
          `${path}/disable?expected_version=${current.version}`,
          {
            method: "POST",
            headers: {
              "X-CSRF-Token": csrf,
              "Idempotency-Key": `b11c-revoke-${id}`,
            },
          },
        );
        return { status: response.status, data: await response.json() };
      },
      { path: grantPath, csrf, id },
    );
    expect(result.status).toBe(200);
    disabled = true;
    adminCsrf = await login(admin);
    await expect
      .poll(
        async () =>
          admin.evaluate(
            async (id) =>
              (
                await (
                  await fetch(`/api/v1/enterprise/remote-access-sessions/${id}`)
                ).json()
              ).status,
            id,
          ),
        { timeout: 60000 },
      )
      .toBe("invalidated");
    const denied = await admin.evaluate(
      async ({ id, csrf }) => {
        const response = await fetch(
          `/api/v1/enterprise/remote-access-sessions/${id}/tickets`,
          {
            method: "POST",
            headers: {
              "X-CSRF-Token": csrf,
              "Idempotency-Key": `b11c-revoked-${id}`,
            },
          },
        );
        return { status: response.status, body: await response.json() };
      },
      { id, csrf: adminCsrf },
    );
    expect(denied.status).toBeGreaterThanOrEqual(400);
    expect(denied.status).toBeLessThan(500);
    const hosts = await admin.evaluate(
      async () =>
        (await (await fetch("/api/v1/enterprise/hosts")).json()).items as {
          id: string;
          connection_status: string;
        }[],
    );
    for (const host of Object.values(mapping.hosts) as { host_id: string }[])
      expect(
        hosts.find((row) => row.id === host.host_id)?.connection_status,
      ).toBe("online");
    writeFileSync(
      info.outputPath("revocation-evidence.json"),
      JSON.stringify(
        {
          session_id: id,
          invalidated: true,
          old_session_ticket_denied: denied.status,
          error_code: denied.body.code,
          control_paths_stayed_online: true,
        },
        null,
        2,
      ),
    );
    await admin.goto("/remote-sessions");
    await admin
      .getByRole("tab", { name: /^(历史会话|Session History)$/ })
      .click();
    await expect(
      admin
        .getByText("B11c actual grant revocation proof", { exact: true })
        .first(),
    ).toBeVisible();
    await admin.screenshot({
      path: info.outputPath("revocation-history.png"),
      fullPage: true,
    });
  } finally {
    if (disabled) {
      if (!adminCsrf) adminCsrf = await login(admin);
      const restored = await admin.evaluate(
        async ({ path, csrf, id }) => {
          const current = await (await fetch(path)).json();
          const response = await fetch(
            `${path}/enable?expected_version=${current.version}`,
            {
              method: "POST",
              headers: {
                "X-CSRF-Token": csrf,
                "Idempotency-Key": `b11c-restore-${id}`,
              },
            },
          );
          return { status: response.status, body: await response.json() };
        },
        { path: grantPath, csrf: adminCsrf, id },
      );
      expect(restored.status).toBe(200);
      expect(restored.body.status).toBe("enabled");
    }
    await adminContext.close();
  }
});

for (const scenario of scenarios)
  test(`B11c real ${scenario.key} ${scenario.protocol} ${scenario.locale} ${scenario.theme}`, async ({
    page,
  }, info) => {
    test.setTimeout(180000);
    page.setDefaultTimeout(25000);
    const english = scenario.locale === "en-US",
      state = mapping.hosts[scenario.key];
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.setViewportSize({
      width: scenario.theme === "light" ? 1440 : 1920,
      height: 1080,
    });
    await page.addInitScript(({ locale, theme }) => {
      localStorage.setItem("argus.locale", locale);
      localStorage.setItem("argus.theme", theme);
    }, scenario);
    await page.goto("/login");
    await page.locator('input[autocomplete="username"]').fill("kakj");
    await page
      .locator('input[autocomplete="current-password"]')
      .fill(process.env.ARGUS_PLAN9_PASSWORD!);
    await page.locator('form button[type="submit"]').click();
    await expect(page).not.toHaveURL(/\/login/);
    await page.goto("/hosts");
    for (const key of ["bastion", "member"]) {
      await expect(
        page
          .getByRole("link", { name: mapping.hosts[key].name, exact: true })
          .first(),
      ).toBeVisible();
      for (const metric of ["cpu", "memory", "storage"])
        await expect(
          page
            .getByTestId(`host-metric-${metric}-${mapping.hosts[key].host_id}`)
            .locator("svg"),
        ).toHaveCount(1, { timeout: 40000 });
    }
    for (const width of [1280, 1440, 1920]) {
      await page.setViewportSize({ width, height: 1080 });
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth + 1,
        ),
      ).toBe(true);
      await page.screenshot({
        path: info.outputPath(`scope-list-${width}.png`),
        fullPage: true,
      });
    }
    for (const board of Object.values(mapping.dashboards) as { id: string }[]) {
      const request = page.waitForResponse(
        (response) =>
          response.url().includes(`/dashboards/${board.id}/execute`) &&
          response.request().method() === "POST",
      );
      await page.goto(`/dashboards/${board.id}?resource=${state.host_id}`);
      const response = await request;
      expect(response.status()).toBe(200);
      const data = await response.json();
      expect(data.panels.length).toBeGreaterThan(0);
      expect(
        data.panels.filter(
          (panel: { status: string }) => panel.status !== "success",
        ),
      ).toEqual([]);
      await expect(page.locator("[data-panel-id]")).toHaveCount(
        data.panels.length,
      );
      await expect(
        page.locator(".argus-observation-chart canvas").first(),
      ).toBeVisible();
      await page.screenshot({
        path: info.outputPath(`dashboard-${board.id}.png`),
        fullPage: true,
      });
    }
    await page.goto(`/hosts/${state.host_id}`);
    await expect(
      page.getByText(state.name, { exact: true }).first(),
    ).toBeVisible();
    if (scenario.key === "member")
      await expect(
        page.getByText(english ? "Control path" : "控制路径", { exact: true }),
      ).toBeVisible();
    await page
      .getByRole("tab", { name: /终端与会话|Terminal & Sessions/ })
      .click();
    const output: string[] = [],
      frames: { type: string; mode?: string; session_id?: string }[] = [];
    page.on("websocket", (socket) => {
      expect(socket.url()).not.toContain("ticket=");
      socket.on("framereceived", (event) => {
        try {
          const frame = JSON.parse(String(event.payload));
          if (frame.type === "output") output.push(frame.data);
          else
            frames.push({
              type: frame.type,
              mode: frame.mode,
              session_id: frame.session_id,
            });
        } catch {
          /* Ignore unrelated frames. */
        }
      });
    });
    await page.getByLabel(/^(协议|Protocol)/).click();
    await page
      .getByRole("option", {
        name: scenario.protocol === "ssh" ? "OpenSSH" : "Shell / PTY",
        exact: true,
      })
      .click();
    await page.getByLabel(/^(登录账号|Account)/).click();
    await page.getByRole("option", { name: "root", exact: true }).click();
    const reason = `B11c ${scenario.key} ${scenario.protocol} ${scenario.locale} ${scenario.theme} readonly review`;
    await page.getByLabel(/事由|Reason/).fill(reason);
    await page
      .getByRole("button", { name: /^(建立会话|Open session)$/ })
      .click();
    await expect
      .poll(() => frames.some((frame) => frame.type === "server_ready"), {
        timeout: 45000,
      })
      .toBe(true);
    const ready = frames.find((frame) => frame.type === "server_ready")!;
    expect(ready.mode).toBe(
      scenario.protocol === "ssh" ? "ssh_pty" : "shell_pty",
    );
    const dock = page.locator(".argus-terminal-dock");
    for (const [label, orientation] of [
      [english ? "Dock to left" : "停靠到左侧", "vertical"],
      [english ? "Dock to right" : "停靠到右侧", "vertical"],
      [english ? "Dock to bottom" : "停靠到底部", "horizontal"],
    ]) {
      const button = dock.getByRole("button", { name: label, exact: true });
      await button.click();
      await expect(button).toHaveAttribute("aria-pressed", "true");
      await expect(dock.getByRole("separator")).toHaveAttribute(
        "aria-orientation",
        orientation!,
      );
    }
    const separator = dock.getByRole("separator");
    await separator.focus();
    const previous = Number(await separator.getAttribute("aria-valuenow"));
    await page.keyboard.press("ArrowUp");
    await expect
      .poll(async () => Number(await separator.getAttribute("aria-valuenow")))
      .toBeGreaterThan(previous);
    const terminal = dock.locator(".argus-terminal__xterm");
    await terminal.click();
    await page.keyboard.type(
      "printf 'B11c-proof-start\\n'; hostname; id; ps -eo comm | head -8; printf 'B11c-proof-end\\n'",
      { delay: 3 },
    );
    await page.keyboard.press("Enter");
    const hostname = `${mapping.run_id}-${scenario.key}`;
    await expect
      .poll(() => output.join(""), { timeout: 30000 })
      .toMatch(
        new RegExp(
          `B11c-proof-start\\r?\\n[\\s\\S]*${hostname}[\\s\\S]*uid=\\d+[\\s\\S]*systemd[\\s\\S]*B11c-proof-end`,
        ),
      );
    const evidence = info.outputPath("terminal-evidence.json");
    writeFileSync(
      evidence,
      JSON.stringify(
        {
          host_id: state.host_id,
          key: scenario.key,
          protocol: scenario.protocol,
          session_id: ready.session_id,
          output: output.join(""),
        },
        null,
        2,
      ),
    );
    await info.attach("real-terminal", {
      path: evidence,
      contentType: "application/json",
    });
    await page.screenshot({
      path: info.outputPath("actual-terminal.png"),
      fullPage: true,
    });
    await dock.getByRole("button", { name: /强制终止|Terminate/ }).click();
    await expect(dock).toBeHidden({ timeout: 35000 });
    await expect
      .poll(
        async () =>
          (
            await (
              await page.request.get(
                `/api/v1/enterprise/remote-access-sessions/${ready.session_id}`,
              )
            ).json()
          ).status,
        { timeout: 35000 },
      )
      .toBe("terminated");
    const session = await (
      await page.request.get(
        `/api/v1/enterprise/remote-access-sessions/${ready.session_id}`,
      )
    ).json();
    expect(session.control_path).toBe(
      scenario.key === "member" ? "bastion_relay" : "executor_tunnel",
    );
    for (const field of ["recording_mode", "command_audit_mode"])
      expect(session[field]).toBe("required");
    for (const field of [
      "file_upload_mode",
      "clipboard_mode",
      "port_forward_mode",
      "session_share_mode",
    ])
      expect(session[field]).toBe("disabled");
    writeFileSync(
      info.outputPath("session.json"),
      JSON.stringify(session, null, 2),
    );
    await page.goto("/remote-sessions");
    await page
      .getByRole("tab", { name: /^(历史会话|Session History)$/ })
      .click();
    const row = page.locator("tbody tr").filter({ hasText: reason }).first();
    const recordingResponse = page.waitForResponse((response) =>
      /\/remote-access-recordings\/[^/]+\/events/.test(response.url()),
    );
    await selectRowAction(row, /查看录像|View recording/);
    const recording = page.getByRole("dialog", {
      name: /会话录像详情|Recording details/,
    });
    expect(
      (await (await recordingResponse).json()).events
        .map((event: { data: string }) => event.data)
        .join(""),
    ).toContain("B11c-proof-end");
    await expect(recording.locator(".xterm-screen")).toBeVisible();
    await recording.getByRole("button", { name: /^(播放|Play)$/ }).click();
    await expect
      .poll(async () =>
        Number(await recording.getByRole("slider").first().inputValue()),
      )
      .toBeGreaterThan(0);
    await page.screenshot({
      path: info.outputPath("recording-replay.png"),
      fullPage: true,
    });
    const alert = mapping.alert_policies.service;
    await page.goto(`/alerts?tab=history&policy_id=${alert.id}`);
    await page.getByText(alert.name, { exact: true }).first().click();
    const detail = page.getByRole("dialog", { name: alert.name });
    await detail.getByRole("tab", { name: /^(时间线|Timeline)$/ }).click();
    await expect(
      detail.locator(".argus-alert-timeline li").first(),
    ).toBeVisible();
    await expect(detail.locator(".argus-alert-timeline pre")).toHaveCount(0);
    await page.screenshot({
      path: info.outputPath("member-alert-history.png"),
      fullPage: true,
    });
    expect(errors).toEqual([]);
  });
