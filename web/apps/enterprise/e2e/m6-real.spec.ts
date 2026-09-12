import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { createMfaLogin, createMfaProof } from "./helpers/mfa-login";

const enabled = process.env.ARGUS_M6_E2E === "1";
const username = process.env.ARGUS_M6_ENTERPRISE_USERNAME ?? "";
const password = process.env.ARGUS_M6_ENTERPRISE_PASSWORD ?? "";
const hostId = process.env.ARGUS_M6_HOST_ID ?? "";
const loginWithMfa = createMfaLogin("enterprise");
const nextEnterpriseMfaProof = createMfaProof("enterprise");

test.describe("M6 Host Connector remote access", () => {
  test.skip(!enabled, "M6 Kubernetes environment is not active");
  test.describe.configure({ mode: "serial", timeout: 120_000 });

  for (const remote of [
    { option: /Shell \/ PTY/i, command: "whoami", output: "argus-connector" },
    { option: /OpenSSH/i, command: "whoami", output: "root" },
  ]) {
    test(`opens ${remote.option} through the outbound Connector`, async ({
      page,
    }) => {
      const websocketURLs: string[] = [];
      page.on("websocket", (socket) => websocketURLs.push(socket.url()));
      await login(page);
      await page.goto(`/hosts/${hostId}`);
      await page.getByRole("tab", { name: /终端与会话|Terminal/i }).click();
      await page.getByLabel(/协议|Protocol/i).click();
      await page.getByRole("option", { name: remote.option }).click();
      await page.getByLabel(/登录账号|Account/i).click();
      await page.getByRole("option", { name: "root" }).click();
      await page
        .getByLabel(/事由|Reason/i)
        .fill("M6 outbound Connector session");
      await page
        .getByRole("button", { name: /建立会话|Open session/i })
        .click();
      const stepUp = page.getByRole("dialog", {
        name: /验证后建立远程会话|Verify before opening/i,
      });
      await expect(stepUp).toBeVisible();
      await stepUp
        .getByLabel(/验证码或恢复码|Authenticator or recovery code/i)
        .fill(await nextEnterpriseMfaProof());
      await stepUp
        .getByRole("button", { name: /验证并继续|Verify and continue/i })
        .click();

      const dock = page.getByRole("region", {
        name: /远程终端|Remote terminal/i,
      });
      await expect(dock).toBeVisible({ timeout: 30_000 });
      const terminal = dock.locator(".argus-terminal__xterm");
      await terminal.click();
      await page.keyboard.type(remote.command);
      await page.keyboard.press("Enter");
      await expect(terminal).toContainText(remote.output, { timeout: 30_000 });
      expect(
        websocketURLs.some(
          (url) => url.includes("/v1/sessions/") && !url.includes("ticket="),
        ),
      ).toBe(true);
      await dock.getByRole("button", { name: /强制终止|Terminate/i }).click();
      await expect(dock).toBeHidden({ timeout: 30_000 });
    });
  }

  for (const locale of ["zh-CN", "en-US"] as const) {
    for (const theme of ["light", "dark"] as const) {
      test(`remote access a11y: ${locale} ${theme}`, async ({ page }) => {
        await page.addInitScript(
          ({ nextLocale, nextTheme }) => {
            window.localStorage.setItem("argus.locale", nextLocale);
            window.localStorage.setItem("argus.theme", nextTheme);
          },
          { nextLocale: locale, nextTheme: theme },
        );
        await login(page);
        await page.goto(`/hosts/${hostId}`);
        await page.getByRole("tab", { name: /终端与会话|Terminal/i }).click();
        await expect(page.locator("html")).toHaveAttribute("lang", locale);
        await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
        const result = await new AxeBuilder({ page }).analyze();
        expect(
          result.violations.filter(
            (violation) =>
              violation.impact === "serious" || violation.impact === "critical",
          ),
        ).toEqual([]);
      });
    }
  }
});

async function login(page: Page) {
  expect(username).not.toBe("");
  expect(password).not.toBe("");
  expect(hostId).not.toBe("");
  await loginWithMfa(page, "/login", username, password);
}
