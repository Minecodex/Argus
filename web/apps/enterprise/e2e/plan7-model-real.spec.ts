import { test, expect } from "@playwright/test";
import { plan7Login } from "./helpers/plan7-login";

const login = plan7Login;
test.describe("Plan7 actual model execution history", () => {
  test.skip(
    process.env.ARGUS_PLAN7_E2E !== "1",
    "Requires the owned Plan7 deployment",
  );
  test.setTimeout(180000);
  test("keeps concurrent context private and shows actual repair and recovery records", async ({
    page,
  }, info) => {
    await page.addInitScript(() =>
      localStorage.setItem("argus.locale", "zh-CN"),
    );
    const projected: string[] = [];
    page.on("response", async (response) => {
      if (
        response.url().includes("/conversations/") &&
        response.headers()["content-type"]?.includes("application/json")
      ) {
        try {
          projected.push(await response.text());
        } catch {
          /* A navigation may cancel an unfinished read. */
        }
      }
    });
    await login(
      page,
      "/login",
      process.env.ARGUS_PLAN7_USERNAME!,
      process.env.ARGUS_PLAN7_PASSWORD!,
    );
    await page.goto(`/?c=${process.env.ARGUS_PLAN7_ACTIVITY_CHAT}`);
    await expect(page.locator(".argus-command-plan").first()).toContainText(
      "执行完成",
    );
    await expect(page.locator("body")).not.toContainText(
      process.env.ARGUS_PLAN7_OTHER_EXECUTION!,
    );
    await expect(page.locator("body")).not.toContainText(
      "argus.remote_activity/v1",
    );
    await expect.poll(() => projected.length).toBeGreaterThan(0);
    expect(
      projected.some((text) =>
        /argus\.remote_activity\/v1|concurrent_activity/.test(text),
      ),
    ).toBe(false);
    await page.screenshot({
      path: info.outputPath("plan7-private-concurrency.png"),
      fullPage: true,
    });
    await page.goto(`/?c=${process.env.ARGUS_PLAN7_REAL_REPAIR_CHAT}`);
    await expect(
      page
        .locator(".argus-command-plan")
        .filter({ hasText: "执行完成" })
        .first(),
    ).toBeVisible();
    await expect(
      page
        .locator(".argus-command-plan")
        .filter({ hasText: "已被新版本替代" })
        .first(),
    ).toBeVisible();
    await page.screenshot({
      path: info.outputPath("plan7-real-model-repair.png"),
      fullPage: true,
    });
    await page.goto(`/?c=${process.env.ARGUS_PLAN7_REAL_RECOVERY_CHAT}`);
    await expect(
      page
        .locator(".argus-command-plan")
        .filter({ hasText: "恢复原执行：" })
        .first(),
    ).toContainText("执行完成");
    await expect(
      page
        .locator(".argus-command-plan")
        .filter({ hasText: "执行结果未知" })
        .first(),
    ).toBeVisible();
    await page.screenshot({
      path: info.outputPath("plan7-real-model-recovery.png"),
      fullPage: true,
    });
  });
});
