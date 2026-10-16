import { selectFirstRowAction } from "./helpers/action-menu";
import { test, expect } from "@playwright/test";
import { setChoice } from "./helpers/choice";
import AxeBuilder from "@axe-core/playwright";
const enabled = process.env.ARGUS_PLAN6_E2E !== "1";
test.describe("Plan6 alert center", () => {
  test.skip(!enabled, "real suite runs separately");
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() =>
      localStorage.setItem("argus.locale", "zh-CN"),
    );
    await page.goto("/login");
    await page
      .getByRole("textbox", { name: "用户名", exact: true })
      .fill("root");
    await page
      .getByRole("textbox", { name: "密码", exact: true })
      .fill("123456");
    await page.getByRole("button", { name: "登录", exact: true }).click();
    await expect(page).not.toHaveURL(/\/login/);
    await page.goto("/alerts");
    await expect(
      page.getByRole("heading", { name: "告警中心", exact: true }),
    ).toBeVisible();
  });
  test("six tabs, filtering, acknowledgement and history", async ({ page }) => {
    await expect(page.getByRole("tab")).toHaveCount(6);
    await expect(
      page.getByText("CPU 使用率持续过高", { exact: true }).first(),
    ).toBeVisible();
    await page.getByPlaceholder("搜索策略、指标或资源…").fill("payment");
    await expect(page.locator("tbody tr")).toHaveCount(1);
    await selectFirstRowAction(page, "确认");
    await expect(
      page.locator("tbody").getByText("已确认", { exact: true }),
    ).toBeVisible();
    await page.getByRole("tab", { name: "告警历史", exact: true }).click();
    await page.getByRole("button", { name: /全部生命周期/ }).click();
    await page.getByRole("option", { name: "已恢复", exact: true }).click();
    await expect(
      page.getByText("数据盘容量告警已恢复", { exact: true }).first(),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "导出记录", exact: true }),
    ).toBeEnabled();
    await page.getByRole("tab", { name: "通知记录", exact: true }).click();
    await expect(
      page.locator("tbody").getByText("发送失败", { exact: true }),
    ).toBeVisible();
    await page.getByRole("tab", { name: "屏蔽与抑制", exact: true }).click();
    await expect(
      page.getByText("数据库磁盘扩容", { exact: true }),
    ).toBeVisible();
    await setChoice(page.getByRole("radio", { name: "抑制规则", exact: true }));
    await expect(
      page.getByText("严重告警抑制警告", { exact: true }),
    ).toBeVisible();
    await page.getByRole("tab", { name: "通知配置", exact: true }).click();
    await expect(page.getByText("生产事件群", { exact: true })).toBeVisible();
    await expect(
      page.getByRole("radio", { name: "路由与合并", exact: true }),
    ).toBeVisible();
  });
  test("multi-rule changes require one reviewable user commit", async ({
    page,
  }) => {
    await page
      .getByRole("button", { name: "创建告警策略", exact: true })
      .first()
      .click();
    const modal = page.getByRole("dialog").filter({
      has: page.getByRole("heading", { name: "创建告警策略", exact: true }),
    });
    await expect(
      modal.getByRole("checkbox", { name: "生产事件群", exact: true }),
    ).toBeChecked();
    await expect(
      modal.getByRole("checkbox", { name: "基础设施邮件", exact: true }),
    ).not.toBeChecked();
    await modal
      .getByRole("textbox", { name: "名称", exact: true })
      .fill("端到端双指标策略");
    await modal
      .getByRole("button", { name: "添加指标规则", exact: true })
      .click();
    await expect(
      modal.getByRole("spinbutton", { name: "阈值", exact: true }),
    ).toHaveCount(2);
    await modal
      .getByRole("textbox", { name: "指标 / PromQL", exact: true })
      .nth(1)
      .fill("host_memory_usage_percent");
    await modal.getByRole("button", { name: "预览变更", exact: true }).click();
    await expect(
      page
        .getByRole("dialog", { name: "预览变更", exact: true })
        .getByText("生产事件群", { exact: true }),
    ).toBeVisible();
    await expect(
      page
        .getByRole("dialog", { name: "预览变更", exact: true })
        .getByText("host_memory_usage_percent > 85%"),
    ).toBeVisible();
    await expect(
      page.locator("tbody").getByText("端到端双指标策略"),
    ).toHaveCount(0);
    await page
      .getByRole("dialog", { name: "预览变更", exact: true })
      .getByRole("button", { name: "取消", exact: true })
      .click();
    await expect(
      modal.getByRole("checkbox", { name: "生产事件群", exact: true }),
    ).toBeChecked();
    await expect(
      modal.getByRole("spinbutton", { name: "阈值", exact: true }),
    ).toHaveCount(2);
    await modal.getByRole("button", { name: "预览变更", exact: true }).click();
    await page.getByRole("button", { name: "确认提交", exact: true }).click();
    await page.getByRole("tab", { name: "告警策略", exact: true }).click();
    await expect(
      page.getByRole("button", { name: "端到端双指标策略", exact: true }),
    ).toBeVisible();
    const issues = (
      await new AxeBuilder({ page })
        .include('[data-testid="alert-center"]')
        .analyze()
    ).violations.filter((v) =>
      ["serious", "critical"].includes(v.impact ?? ""),
    );
    expect(issues).toEqual([]);
  });
  test("evaluation and audit stay in Organization and Access", async ({
    page,
  }) => {
    await page.getByRole("link", { name: "组织与权限", exact: true }).click();
    await page.getByRole("tab", { name: "告警评估", exact: true }).click();
    await expect(
      page.getByRole("button", { name: "后台评估账号", exact: true }),
    ).toBeVisible();
    await page.getByRole("tab", { name: "告警审计", exact: true }).click();
    await expect(
      page.getByRole("heading", { name: "告警审计", exact: true }),
    ).toBeVisible();
  });
  test("channel defaults are reviewable and existing policy targets survive editing", async ({
    page,
  }) => {
    await page.getByRole("tab", { name: "通知配置", exact: true }).click();
    // Turn the only default off through its ordinary configuration workflow.
    await page
      .locator(".argus-alert-panel")
      .filter({ hasText: "生产事件群" })
      .getByRole("button", { name: "配置", exact: true })
      .click();
    const channel = page.getByRole("dialog");
    await channel.getByText("凭据与提醒选项", { exact: true }).click();
    const choice = channel.getByRole("checkbox", {
      name: "新建策略默认选用此渠道",
      exact: true,
    });
    await expect(choice).toBeChecked();
    await choice.uncheck();
    await channel
      .getByRole("button", { name: "预览变更", exact: true })
      .click();
    await expect(
      page
        .getByRole("dialog", { name: "预览变更", exact: true })
        .getByText("新建策略默认选用此渠道", { exact: true }),
    ).toBeVisible();
    await page.getByRole("button", { name: "确认提交", exact: true }).click();
    await page
      .getByRole("button", { name: "创建告警策略", exact: true })
      .first()
      .click();
    await expect(
      page
        .getByRole("dialog")
        .getByRole("checkbox", { name: "生产事件群", exact: true }),
    ).not.toBeChecked();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "关闭", exact: true })
      .click();
    await page.getByRole("tab", { name: "告警策略", exact: true }).click();
    await selectFirstRowAction(page, "编辑");
    await expect(
      page
        .getByRole("dialog")
        .getByRole("checkbox", { name: "生产事件群", exact: true }),
    ).toBeChecked();
    await expect(
      page
        .getByRole("dialog")
        .getByRole("checkbox", { name: "基础设施邮件", exact: true }),
    ).toBeChecked();
  });
});
