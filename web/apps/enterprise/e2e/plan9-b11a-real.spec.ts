import { expect, test } from "@playwright/test";
import { readFileSync, writeFileSync } from "node:fs";
import { selectRowAction } from "./helpers/action-menu";

const enabled = process.env.ARGUS_PLAN9_B11A_REAL === "1";
const mapping = enabled
  ? JSON.parse(readFileSync(process.env.ARGUS_PLAN9_B11A_MAP!, "utf8"))
  : {};
test.skip(!enabled, "Requires the real owned B11a direct host");
test.use({
  trace: "off",
  video: "off",
  ignoreHTTPSErrors: false,
  reducedMotion: "reduce",
});

for (const locale of ["zh-CN", "en-US"])
  for (const theme of ["light", "dark"]) {
    test(`B11a direct host ${locale} ${theme}`, async ({ page }, info) => {
      test.setTimeout(240_000);
      page.setDefaultTimeout(25000);
      await page.setViewportSize({
        width: theme === "light" ? 1280 : 1920,
        height: 1080,
      });
      await page.addInitScript(
        ({ locale, theme }) => {
          localStorage.setItem("argus.locale", locale);
          localStorage.setItem("argus.theme", theme);
        },
        { locale, theme },
      );
      const errors: string[] = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.goto("/login");
      await page.locator('input[autocomplete="username"]').fill("kakj");
      await page
        .locator('input[autocomplete="current-password"]')
        .fill(process.env.ARGUS_PLAN9_PASSWORD!);
      await page.locator('form button[type="submit"]').click();
      await expect(page).not.toHaveURL(/\/login/);
      const host = mapping.hosts.direct.host_id;
      await page.goto(`/hosts/${host}`);
      await expect(
        page.getByText("B11a-Linux-Direct", { exact: true }).first(),
      ).toBeVisible();
      await page
        .getByRole("tab", { name: /组件与采集|Components & Collection/ })
        .click();
      // The fifth definition is Windows services, unavailable for this Linux host.
      await expect(page.locator("tbody tr")).toHaveCount(5);
      await page.screenshot({
        path: info.outputPath("collections.png"),
        fullPage: true,
      });
      for (const board of Object.values(mapping.dashboards) as {
        id: string;
        name: string;
      }[]) {
        const response = page.waitForResponse(
          (r) =>
            r.url().includes(`/dashboards/${board.id}/execute`) &&
            r.request().method() === "POST",
        );
        await page.goto(`/dashboards/${board.id}?resource=${host}`);
        const actual = await response;
        expect(actual.status()).toBe(200);
        const data = await actual.json();
        expect(data.panels.length).toBeGreaterThan(0);
        expect(
          data.panels.filter((p: { status: string }) => p.status !== "success"),
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
      await page.goto(`/hosts/${host}`);
      await page
        .getByRole("tab", { name: /终端与会话|Terminal & Sessions/ })
        .click();
      const output: string[] = [];
      const frames: {
        type: string;
        mode?: string;
        session_id?: string;
        code?: string;
      }[] = [];
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
                code: frame.code,
              });
          } catch {
            /* Non-remote websocket frames do not prove terminal output. */
          }
        });
      });
      const ssh = theme === "dark";
      await page.getByLabel(/^(协议|Protocol)/).click();
      await page
        .getByRole("option", {
          name: ssh ? "OpenSSH" : "Shell / PTY",
          exact: true,
        })
        .click();
      await page.getByLabel(/^(登录账号|Account)/).click();
      await page.getByRole("option", { name: "root", exact: true }).click();
      const reason = `B11a real ${ssh ? "SSH" : "Shell"} read-only review`;
      await page.getByLabel(/事由|Reason/).fill(reason);
      await page
        .getByRole("button", { name: /^(建立会话|Open session)$/ })
        .click();
      await expect
        .poll(() => frames.some((f) => f.type === "server_ready"), {
          timeout: 45000,
        })
        .toBe(true);
      const dock = page.locator(".argus-terminal-dock");
      const terminal = dock.locator(".argus-terminal__xterm");
      await expect(terminal.locator(".xterm-helper-textarea")).toBeAttached();
      await terminal.click();
      const command =
        "printf 'B11a-proof-start\\n'; hostname; id; ps -eo comm | head -8; printf 'B11a-proof-end\\n'";
      await page.keyboard.type(command, { delay: 3 });
      await page.keyboard.press("Enter");
      // A prompt echo is insufficient: assert real identity/process output between markers.
      await expect
        .poll(() => output.join(""), { timeout: 30000 })
        .toMatch(
          /B11a-proof-start\r?\n.*p9-b11a-20261113-01[\s\S]*uid=\d+[\s\S]*systemd[\s\S]*B11a-proof-end/,
        );
      const ready = frames.find((f) => f.type === "server_ready")!;
      expect(ready.mode).toBe(ssh ? "ssh_pty" : "shell_pty");
      const evidence = info.outputPath("terminal-evidence.json");
      writeFileSync(
        evidence,
        JSON.stringify(
          {
            protocol: ssh ? "ssh" : "shell",
            session_id: ready.session_id,
            output: output.join(""),
          },
          null,
          2,
        ),
      );
      await info.attach("terminal-evidence", {
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
          async () => {
            const response = await page.request.get(
              `/api/v1/enterprise/remote-access-sessions/${ready.session_id}`,
            );
            return response.ok()
              ? (await response.json()).status
              : "unavailable";
          },
          { timeout: 35000 },
        )
        .toBe("terminated");
      const session = await page.request.get(
        `/api/v1/enterprise/remote-access-sessions/${ready.session_id}`,
      );
      expect(session.status()).toBe(200);
      const detail = await session.json();
      expect(detail.status).toBe("terminated");
      const sessionEvidence = info.outputPath("session.json");
      writeFileSync(sessionEvidence, JSON.stringify(detail, null, 2));
      await info.attach("session", {
        path: sessionEvidence,
        contentType: "application/json",
      });
      await page.goto("/remote-sessions");
      await page
        .getByRole("tab", { name: /^(历史会话|Session History)$/ })
        .click();
      const row = page.locator("tbody tr").filter({ hasText: reason }).first();
      const recordingResponse = page.waitForResponse((r) =>
        /\/remote-access-recordings\/[^/]+\/events/.test(r.url()),
      );
      await selectRowAction(row, /查看录像|View recording/);
      const recording = page.getByRole("dialog", {
        name: /会话录像详情|Recording details/,
      });
      const recordingData = await (await recordingResponse).json();
      expect(
        recordingData.events
          .map((event: { data: string }) => event.data)
          .join(""),
      ).toContain("B11a-proof-end");
      await expect(
        recording.locator(".argus-terminal-player__screen .xterm-screen"),
      ).toBeVisible();
      await recording.getByRole("button", { name: /^(播放|Play)$/ }).click();
      await expect
        .poll(
          async () =>
            Number(await recording.getByRole("slider").first().inputValue()),
          { timeout: 10000 },
        )
        .toBeGreaterThan(0);
      await page.screenshot({
        path: info.outputPath("recording-replay.png"),
        fullPage: true,
      });
      const alert = mapping.alert_policies.service;
      await page.goto(`/alerts?tab=history&policy_id=${alert.id}`);
      await page.getByText(alert.name, { exact: true }).first().click();
      const alertDetail = page.getByRole("dialog", { name: alert.name });
      await alertDetail
        .getByRole("tab", { name: /^(时间线|Timeline)$/ })
        .click();
      await expect(
        alertDetail.locator(".argus-alert-timeline li").first(),
      ).toBeVisible();
      await expect(
        alertDetail.locator(".argus-alert-timeline pre"),
      ).toHaveCount(0);
      await page.screenshot({
        path: info.outputPath("recovered-alert.png"),
        fullPage: true,
      });
      expect(errors).toEqual([]);
      await page.request.delete("/api/v1/enterprise/auth/session");
    });
  }
