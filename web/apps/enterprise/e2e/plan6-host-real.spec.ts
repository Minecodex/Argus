import { selectFirstRowAction } from "./helpers/action-menu";
import { test, expect } from "@playwright/test";
import { createMfaLogin } from "./helpers/mfa-login";
const enabled = process.env.ARGUS_PLAN6_HOST_E2E === "1";
const login = createMfaLogin("enterprise");
test.describe("Docker Host via Bastion monitoring loop", () => {
  test.skip(!enabled, "owned real Docker Host acceptance is required");
  test.describe.configure({ mode: "default", timeout: 120_000 });
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() =>
      localStorage.setItem("argus.locale", "zh-CN"),
    );
    await login(
      page,
      "/login",
      process.env.ARGUS_PLAN6_USERNAME ?? "",
      process.env.ARGUS_PLAN6_PASSWORD ?? "",
    );
  });
  test("Host Collector and its real system metric dashboard", async ({
    page,
  }, info) => {
    const host = process.env.ARGUS_PLAN6_HOST_ID!;
    await page.goto(`/hosts/${host}`);
    await expect(
      page
        .getByText(process.env.ARGUS_PLAN6_HOST_NAME!, { exact: true })
        .first(),
    ).toBeVisible();
    await page.getByRole("tab", { name: /组件|Components/ }).click();
    await expect(page.getByText(/监控中|Monitoring/).first()).toBeVisible();
    const id = process.env.ARGUS_PLAN6_HOST_DASHBOARD_ID!;
    const execution = page.waitForResponse(
      (r) =>
        r.url().includes(`/dashboards/${id}/execute`) && r.status() === 200,
    );
    await page.goto(`/dashboards/${id}?resource=${host}`);
    const data = await (await execution).json();
    expect(data.panels).toHaveLength(2);
    expect(
      data.panels.every((p: { status: string }) => p.status === "success"),
    ).toBeTruthy();
    await expect(page.locator("[data-panel-id]")).toHaveCount(2);
    await expect(
      page.getByText("CPU usage", { exact: true }).first(),
    ).toBeVisible();
    await expect(
      page.getByText("Memory usage", { exact: true }).first(),
    ).toBeVisible();
    await page.screenshot({
      path: info.outputPath("real-docker-host-dashboard.png"),
      fullPage: true,
    });
  });
  test("recovery history and actual notification records", async ({
    page,
  }, info) => {
    const policy = process.env.ARGUS_PLAN6_HOST_POLICY_ID!;
    await page.goto(`/alerts?tab=history&policy_id=${policy}`);
    await expect(
      page.locator("tbody").getByText("已恢复", { exact: true }).first(),
    ).toBeVisible();
    await selectFirstRowAction(page, "详情");
    await expect(
      page
        .getByRole("dialog")
        .getByText("Real Docker Host alert loop verified"),
    ).toBeVisible();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "关闭", exact: true })
      .click();
    await page.getByRole("tab", { name: "通知记录", exact: true }).click();
    await expect(
      page.locator("tbody").getByText("已受理", { exact: true }).first(),
    ).toBeVisible();
    await page.screenshot({
      path: info.outputPath("real-docker-host-notifications.png"),
      fullPage: true,
    });
  });
  test("real GLM Host alerts and dashboard file analysis remain visible", async ({
    page,
  }, info) => {
    for (const id of [
      process.env.ARGUS_PLAN6_HOST_ALERT_CONVERSATION,
      process.env.ARGUS_PLAN6_HOST_DASHBOARD_CONVERSATION,
    ]) {
      expect(id).toBeTruthy();
      await page.goto(`/?c=${id}`);
      await expect(page.getByTestId("tool-trace").first()).toBeVisible();
      await expect(
        page.getByTestId("chat-message-assistant").last(),
      ).toBeVisible();
      await expect(
        page.getByRole("button", { name: "停止生成", exact: true }),
      ).toHaveCount(0);
    }
    await page.screenshot({
      path: info.outputPath("real-glm-host-dashboard-analysis.png"),
      fullPage: true,
    });
  });
});
