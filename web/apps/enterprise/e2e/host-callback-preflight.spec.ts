import { expect, test } from "@playwright/test";

for (const [language, theme] of [
  ["zh-CN", "light"],
  ["en-US", "dark"],
] as const) {
  test(`SSH callback failure blocks preview with a specific diagnosis (${language}, ${theme})`, async ({
    page,
  }, info) => {
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    page.on("console", (message) => {
      if (message.type() === "error") errors.push(message.text());
    });
    // Replace only the simulated backend response; the actual form, request mapper,
    // name availability checks, polling, and preview gate run unchanged.
    await page.route("**/mock/host-connection-tests.ts*", async (route) => {
      const url = new URL(route.request().url());
      if (url.searchParams.has("callback-fixture-original"))
        return route.continue();
      url.searchParams.set("callback-fixture-original", "1");
      await route.fulfill({
        contentType: "application/javascript",
        body: `
        import { createHostConnectionTests as original } from ${JSON.stringify(url.href)};
        export function createHostConnectionTests(ctx) {
          const tests = original(ctx);
          return { ...tests,
            async create(input) {
              window.__callbackTestRequest = structuredClone(input);
              const result = await tests.create(input);
              return { ...result, status: "failed", error_code: "HOST_ONBOARDING_CALLBACK_DNS_FAILED",
                checks: [{ name: "authentication", status: "passed" }, { name: "onboarding_callback", status: "failed" }] };
            },
            requireOnboarding(input) { window.__callbackPreviewAttempted = true; return tests.requireOnboarding(input); }
          };
        }
      `,
      });
    });
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
    const zh = language === "zh-CN";
    const title = zh ? "添加普通主机" : "Add Host";
    await page.getByRole("button", { name: title, exact: true }).click();
    const dialog = page.getByRole("dialog", { name: title, exact: true });
    await dialog
      .locator(".argus-scenario-card")
      .filter({
        hasText: zh
          ? "平台可 SSH · 主机无出站"
          : "Platform SSH · no host egress",
      })
      .click();
    await dialog
      .getByRole("button", { name: zh ? "下一步" : "Next", exact: true })
      .click();
    await dialog
      .getByRole("textbox", { name: zh ? "主机名" : "Host name", exact: true })
      .fill("callback-e2e-host");
    await dialog
      .getByRole("textbox", { name: zh ? "地址" : "Address", exact: true })
      .fill("192.0.2.10");
    await dialog
      .getByRole("textbox", {
        name: zh ? "登录账号" : "Login account",
        exact: true,
      })
      .fill("root");
    await dialog
      .getByRole("button", {
        name: zh ? "SSH 凭据" : "SSH credential",
        exact: true,
      })
      .click();
    await page.getByRole("option").first().click();
    await dialog.locator('button[type="submit"]').click();
    await expect(
      dialog.getByText(/HOST_ONBOARDING_CALLBACK_DNS_FAILED/),
    ).toBeVisible();
    await expect(
      dialog.getByText(/HOST_ONBOARDING_CALLBACK_DNS_FAILED/),
    ).toBeInViewport();
    await expect(
      dialog.getByText(
        zh
          ? /目标主机无法解析平台回调地址/
          : /The target host cannot resolve the platform callback address/,
      ),
    ).toBeVisible();
    await expect(
      dialog.getByText(
        zh ? "SSH 身份验证: 通过" : "SSH authentication: Passed",
        { exact: true },
      ),
    ).toBeVisible();
    await expect(
      dialog.getByText(
        zh
          ? "目标主机访问平台回调: 失败"
          : "Target host to platform callback: Failed",
        { exact: true },
      ),
    ).toBeVisible();
    await expect(dialog.getByTestId("host-onboarding-flow")).toHaveAttribute(
      "data-phase",
      "details",
    );
    await expect(
      dialog.getByRole("button", {
        name: zh ? "确认执行" : "Confirm",
        exact: true,
      }),
    ).toHaveCount(0);
    const evidence = await page.evaluate(() => {
      const state = window as typeof window & {
        __callbackTestRequest?: Record<string, unknown>;
        __callbackPreviewAttempted?: boolean;
      };
      return {
        request: state.__callbackTestRequest,
        preview: Boolean(state.__callbackPreviewAttempted),
      };
    });
    expect(evidence.request).toMatchObject({
      onboarding_control_path: "executor_tunnel",
      ssh_path: "direct_executor",
    });
    expect(evidence.preview).toBe(false);
    await expect(page.locator("vite-error-overlay")).toHaveCount(0);
    expect(errors).toEqual([]);
    await page.screenshot({ path: info.outputPath("callback-preflight.png") });
  });
}
