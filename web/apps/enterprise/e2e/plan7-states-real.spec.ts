import { test, expect } from "@playwright/test";
import { plan7Login } from "./helpers/plan7-login";

const login = plan7Login;
test.describe("Plan7 authoritative exception and recovery cards", () => {
  test.skip(
    process.env.ARGUS_PLAN7_E2E !== "1",
    "Requires the owned Plan7 deployment",
  );
  test.setTimeout(180000);
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
  test("shows each target, identity and complete native script before one approval", async ({
    page,
  }) => {
    await page.goto(`/?c=${process.env.ARGUS_PLAN7_REVIEW_CHAT}`);
    await expect(page.locator(".argus-command-plan").first()).toBeVisible();
    const plan = page.locator(".argus-command-plan").first();
    await expect(plan).toContainText("printf 'Linux review");
    await expect(plan).toContainText("Write-Output 'Windows review'");
    await expect(plan).toContainText("S-1-");
    await expect(plan.locator(".argus-command-plan__steps > li")).toHaveCount(
      2,
    );
    await expect(
      plan.getByRole("button", { name: "确认执行", exact: true }),
    ).toHaveCount(1);
    await plan.getByRole("button", { name: "确认执行", exact: true }).click();
    await expect(plan).toContainText("执行完成", { timeout: 90000 });
    await expect(plan).toContainText("Windows review");
  });
  test("does not offer confirmation for expired server previews", async ({
    page,
  }) => {
    await page.goto(`/?c=${process.env.ARGUS_PLAN7_EXPIRED_CHAT}`);
    await expect(page.locator(".argus-command-plan").first()).toBeVisible();
    const plan = page.locator(".argus-command-plan").first();
    await expect(plan).toContainText("确认已过期");
    await expect(plan).toContainText("expired must not execute");
    await expect(
      plan.getByRole("button", { name: "确认执行", exact: true }),
    ).toHaveCount(0);
    await page.reload();
    await expect(page.locator(".argus-command-plan").first()).toBeVisible();
    await expect(page.locator(".argus-command-plan").first()).toContainText(
      "确认已过期",
    );
  });
  test("keeps successful, failed and skipped steps visible after partial failure", async ({
    page,
  }) => {
    await page.goto(`/?c=${process.env.ARGUS_PLAN7_FAILURE_CHAT}`);
    await expect(page.locator(".argus-command-plan").first()).toBeVisible();
    const plan = page.locator(".argus-command-plan").first();
    await expect(plan).toContainText("部分完成后失败");
    const steps = plan.locator(".argus-command-plan__steps > li");
    await expect(steps.nth(0)).toContainText("执行完成");
    await expect(steps.nth(1)).toContainText("执行失败");
    await expect(steps.nth(1)).toContainText("expected failure");
    await expect(steps.nth(2)).toContainText("未执行");
    await expect(
      plan.getByRole("button", { name: "确认执行", exact: true }),
    ).toHaveCount(0);
  });
  test("shows original lost context and separately approves the linked recovery", async ({
    page,
  }) => {
    await page.goto(`/?c=${process.env.ARGUS_PLAN7_RECOVERY_CHAT}`);
    await expect(page.locator(".argus-command-plan").first()).toBeVisible();
    const original = page
      .locator(".argus-command-plan")
      .filter({ hasText: "kill -KILL" })
      .first();
    const recovery = page
      .locator(".argus-command-plan")
      .filter({ hasText: "恢复原执行：" })
      .first();
    await expect(original).toContainText("已暂停，等待恢复方案");
    await expect(recovery).toContainText("等待确认");
    await expect(
      original.getByRole("button", { name: "确认执行", exact: true }),
    ).toHaveCount(0);
    await recovery
      .getByRole("button", { name: "确认执行", exact: true })
      .click();
    await expect(recovery).toContainText("执行完成", { timeout: 90000 });
    await expect(original).toContainText("已暂停，等待恢复方案");
    await page.reload();
    await expect(page.locator(".argus-command-plan").first()).toBeVisible();
    await expect(
      page
        .locator(".argus-command-plan")
        .filter({ hasText: "恢复原执行：" })
        .first(),
    ).toContainText("执行完成");
    await expect(
      page
        .locator(".argus-command-plan")
        .filter({ hasText: "kill -KILL" })
        .first(),
    ).toContainText("已暂停，等待恢复方案");
  });
});
