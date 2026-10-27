import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const english of [false, true])
  for (const theme of ["light", "dark"])
    for (const width of [1280, 1920]) {
      test(`Plan8 drain preview and cancel ${english ? "en" : "zh"} ${theme} ${width}`, async ({
        page,
      }, info) => {
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        await page.setViewportSize({ width, height: 1080 });
        await page.addInitScript(
          ({ english, theme }) => {
            localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
            localStorage.setItem("argus.theme", theme);
          },
          { english, theme },
        );
        await page.goto("/login");
        await page.locator('input[autocomplete="username"]').fill("root");
        await page
          .locator('input[autocomplete="current-password"]')
          .fill("123456");
        await page.locator('form button[type="submit"]').click();
        await expect(page).not.toHaveURL(/\/login/);
        await page.goto("/kubernetes/k8s-prod-east");
        await page
          .getByRole("tab", { name: english ? "Cluster" : "集群", exact: true })
          .click();
        await page
          .getByRole("button", {
            name: new RegExp(english ? "Resource type" : "资源类型"),
          })
          .click();
        await page.getByRole("option", { name: "Node", exact: true }).click();
        const row = page.getByRole("row").filter({ hasText: "node-example" });
        await row
          .getByRole("button", {
            name: english ? "Manage object" : "管理对象",
            exact: true,
          })
          .click();
        const dialog = page.getByRole("dialog", {
          name: new RegExp("Node.*node-example"),
        });
        await dialog
          .getByRole("button", {
            name: new RegExp(english ? "Operation" : "操作"),
          })
          .click();
        await page
          .getByRole("option", {
            name: english ? "Drain node" : "维护节点并驱逐 Pod",
            exact: true,
          })
          .click();
        await expect(dialog).toContainText(
          english ? "node remains cordoned" : "保持禁止调度",
        );
        await dialog
          .getByRole("button", {
            name: english ? "Validate and preview" : "验证并预览差异",
            exact: true,
          })
          .click();
        const progress = dialog.getByRole("region", {
          name: english ? "Node maintenance progress" : "节点维护进度",
        });
        await expect(progress).toBeVisible();
        await expect(
          progress.getByRole("cell", { name: "pod-example", exact: true }),
        ).toHaveCount(2);
        await expect(progress).toContainText(
          english ? "Pending eviction" : "待驱逐",
        );
        await expect(progress).not.toContainText("spec_digest");
        expect(
          (await new AxeBuilder({ page }).include('[role="dialog"]').analyze())
            .violations,
        ).toEqual([]);
        await page.screenshot({ path: info.outputPath("drain-preview.png") });
        await dialog
          .getByRole("button", {
            name: english ? "Confirm operation" : "确认执行",
            exact: true,
          })
          .click();
        await expect(dialog).toContainText(
          english ? "dual approval" : "等待审批",
        );
        await dialog
          .getByRole("button", {
            name: english ? "Close" : "关闭",
            exact: true,
          })
          .last()
          .click();
        await page.getByRole("button", { name: /root$/ }).click();
        await page
          .getByRole("menuitem", {
            name: english ? "Sign out" : "退出登录",
            exact: true,
          })
          .click();
        await expect(page).toHaveURL(/\/login/);
        await page.locator('input[autocomplete="username"]').fill("chenxi");
        await page
          .locator('input[autocomplete="current-password"]')
          .fill("123456");
        await page.locator('form button[type="submit"]').click();
        await expect(page).not.toHaveURL(/\/login/);
        await page.goto("/approvals");
        await page.getByRole("button", { name: /node-example/ }).click();
        const approval = page.locator(".argus-approval-detail");
        await approval
          .getByLabel(english ? "Approve" : "批准", { exact: true })
          .fill(
            english ? "Verified node maintenance window" : "已核对节点维护窗口",
          );
        const approve = approval.getByRole("button", {
          name: english ? "Approve" : "批准",
          exact: true,
        });
        await approve.click();
        await expect(approve).not.toBeVisible();
        await page.getByRole("button", { name: /chenxi$/ }).click();
        await page
          .getByRole("menuitem", {
            name: english ? "Sign out" : "退出登录",
            exact: true,
          })
          .click();
        await expect(page).toHaveURL(/\/login/);
        await page.locator('input[autocomplete="username"]').fill("root");
        await page
          .locator('input[autocomplete="current-password"]')
          .fill("123456");
        await page.locator('form button[type="submit"]').click();
        await expect(page).not.toHaveURL(/\/login/);
        await page.goto("/kubernetes/k8s-prod-east");
        await page
          .getByRole("button", {
            name: english ? "Operation history" : "操作记录",
            exact: true,
          })
          .click();
        const history = page.getByRole("dialog", {
          name: english ? "Operation history" : "操作记录",
          exact: true,
        });
        await history
          .getByRole("row")
          .filter({ hasText: "node-example" })
          .getByRole("button", {
            name: english ? "View object" : "查看对象",
            exact: true,
          })
          .click();
        await expect(
          history.getByRole("region", {
            name: english ? "Node maintenance progress" : "节点维护进度",
          }),
        ).toBeVisible();
        await history
          .getByRole("button", {
            name: english ? "Stop further evictions" : "停止后续驱逐",
            exact: true,
          })
          .click();
        await expect(history).toContainText(
          english ? "Further batches stopped" : "后续批次已停止",
        );
        await expect(
          history.getByRole("button", {
            name: english ? "Stop further evictions" : "停止后续驱逐",
            exact: true,
          }),
        ).not.toBeVisible();
        expect(errors).toEqual([]);
      });
    }
