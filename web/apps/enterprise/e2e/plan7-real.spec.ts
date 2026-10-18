import { test, expect } from "@playwright/test";
import { plan7Login } from "./helpers/plan7-login";
import { setChoice } from "./helpers/choice";
import { readFile } from "node:fs/promises";

const enabled = process.env.ARGUS_PLAN7_E2E === "1";
const login = plan7Login;
test.describe("Plan7 real command approvals", () => {
  test.skip(!enabled, "Requires the owned Plan7 deployment");
  test.describe.configure({ mode: "serial", timeout: 180000 });
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() =>
      localStorage.setItem("argus.locale", "zh-CN"),
    );
    await login(
      page,
      "/login",
      process.env.ARGUS_PLAN7_USERNAME!,
      process.env.ARGUS_PLAN7_PASSWORD!,
    );
  });
  test("reviews full commands, persists mode, confirms once and reloads", async ({
    page,
  }) => {
    const conversation = process.env.ARGUS_PLAN7_CONFIRM_CHAT!;
    await page.goto(`/?c=${conversation}`);
    await expect(page.locator(".argus-command-plan").first()).toBeVisible();
    const plan = page.locator(".argus-command-plan").first();
    await expect(plan).toBeVisible();
    await expect(plan).toContainText("browser confirmed");
    const mode = page.getByRole("radio", { name: "计划", exact: true });
    await setChoice(mode);
    const confirmation = page.waitForResponse(
      (r) => r.url().includes("/confirm") && r.request().method() === "POST",
    );
    await plan.getByRole("button", { name: "确认执行", exact: true }).click();
    expect((await confirmation).status()).toBe(200);
    await expect(plan).toContainText("执行完成", { timeout: 90000 });
    await page.reload();
    await expect(page.locator(".argus-command-plan").first()).toBeVisible();
    await expect(
      page.getByRole("radio", { name: "计划", exact: true }),
    ).toBeChecked();
    await expect(page.locator(".argus-command-plan").first()).toContainText(
      "执行完成",
    );
    await expect(
      page
        .locator(".argus-command-plan")
        .getByRole("button", { name: "确认执行", exact: true }),
    ).toHaveCount(0);
    const csv = page
      .locator(".argus-file-card")
      .filter({
        has: page.getByRole("button", {
          name: "remote-table.csv",
          exact: true,
        }),
      });
    await expect(csv).toContainText("text/csv");
    await csv
      .getByRole("button", { name: "remote-table.csv", exact: true })
      .click();
    await expect(page.getByRole("dialog")).toContainText("host-a,42");
    await page.keyboard.press("Escape");
    const binary = page
      .locator(".argus-file-card")
      .filter({
        has: page.getByRole("button", {
          name: "remote-binary.txt",
          exact: true,
        }),
      });
    await expect(binary).toContainText("application/octet-stream");
    await binary
      .getByRole("button", { name: "remote-binary.txt", exact: true })
      .click();
    await expect(page.getByRole("dialog")).toContainText(
      "此格式暂不支持在线预览，请下载查看。",
    );
    await page.keyboard.press("Escape");
    const downloaded = page.waitForEvent("download");
    await binary.getByRole("link", { name: "下载 remote-binary.txt" }).click();
    const download = await downloaded;
    expect(download.suggestedFilename()).toBe("remote-binary.txt");
    expect(await readFile((await download.path())!)).toEqual(
      Buffer.from([0, 1, 255]),
    );
  });
  test("continues across reload and stops only on explicit click", async ({
    page,
  }) => {
    await page.goto(`/?c=${process.env.ARGUS_PLAN7_STOP_CHAT}`);
    await expect(page.locator(".argus-command-plan").first()).toBeVisible();
    await page
      .locator(".argus-command-plan")
      .first()
      .getByRole("button", { name: "确认执行", exact: true })
      .click();
    await expect(
      page
        .locator(".argus-command-plan")
        .first()
        .getByRole("button", { name: "停止执行", exact: true }),
    ).toBeVisible();
    await page.reload();
    await expect(page.locator(".argus-command-plan").first()).toBeVisible();
    const plan = page.locator(".argus-command-plan").first();
    const stopping = page.waitForResponse(
      (r) => r.url().endsWith("/stop") && r.request().method() === "POST",
    );
    await plan.getByRole("button", { name: "停止执行", exact: true }).click();
    expect((await stopping).status()).toBe(200);
    await expect(plan).toContainText("已取消", { timeout: 90000 });
    await expect(
      plan.getByRole("button", { name: "确认执行", exact: true }),
    ).toHaveCount(0);
  });
});

for (const locale of ["zh-CN", "en-US"])
  for (const theme of ["light", "dark"])
    for (const viewport of [
      { width: 1280, height: 800 },
      { width: 1440, height: 900 },
      { width: 1920, height: 1080 },
    ]) {
      test.describe(`Plan7 ${locale} ${theme} ${viewport.width}`, () => {
        test.skip(!enabled, "Requires real Plan7 history");
        test.use({ viewport });
        test.setTimeout(180000);
        test("renders real plans and PowerShell history without overflow", async ({
          page,
        }, info) => {
          await page.addInitScript(
            ({ locale, theme }) => {
              localStorage.setItem("argus.locale", locale);
              localStorage.setItem("argus.theme", theme);
            },
            { locale, theme },
          );
          await login(
            page,
            "/login",
            process.env.ARGUS_PLAN7_USERNAME!,
            process.env.ARGUS_PLAN7_PASSWORD!,
          );
          await page.goto(`/?c=${process.env.ARGUS_PLAN7_LINUX_CHAT}`);
          await expect(
            page.locator(".argus-command-plan").first(),
          ).toBeVisible();
          await expect(page.locator("html")).toHaveAttribute(
            "data-theme",
            theme,
          );
          const shared = page.locator(".argus-command-plan").nth(1);
          await shared
            .getByText(
              locale === "zh-CN" ? "共享会话上下文" : "Shared session context",
              { exact: true },
            )
            .click();
          await expect(shared).toContainText("/tmp/argus/");
          await shared
            .getByText(
              locale === "zh-CN"
                ? "查看前序批次（第 1 步）"
                : "Inspect preceding batch (step 1)",
              { exact: true },
            )
            .click();
          await expect(shared).toContainText("p7_value=41");
          await expect(
            page.getByRole("radiogroup", {
              name: locale === "zh-CN" ? "工作模式" : "Working mode",
            }),
          ).toBeVisible();
          expect(
            await page.evaluate(
              () =>
                document.documentElement.scrollWidth <= window.innerWidth + 2,
            ),
          ).toBe(true);
          await page.screenshot({
            path: info.outputPath(
              `plan7-${locale}-${theme}-${viewport.width}.png`,
            ),
            fullPage: true,
          });
          await page.goto(`/?c=${process.env.ARGUS_PLAN7_WINDOWS_CHAT}`);
          await expect(
            page.locator(".argus-command-plan").first(),
          ).toBeVisible();
          await expect(
            page.locator(".argus-command-plan").first(),
          ).toContainText("powershell");
          await expect(
            page.locator(".argus-command-plan").first(),
          ).toContainText("S-1-");
          expect(
            await page.evaluate(
              () =>
                document.documentElement.scrollWidth <= window.innerWidth + 2,
            ),
          ).toBe(true);
        });
      });
    }
