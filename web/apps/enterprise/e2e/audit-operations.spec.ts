import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

async function seed(page: Page) {
  return page.evaluate(() => {
    const key = Object.keys(localStorage).find((key) =>
      key.startsWith("argus-mock:db-"),
    )!;
    const db = JSON.parse(localStorage.getItem(key)!);
    const host = db.hosts.find(
      (h: { name: string }) => h.name === "public-web-01",
    );
    const enterprise = host.enterpriseId;
    const operation = {
      action_ref: "act-audit-failed",
      action_type: "host.update",
      action_status: "failed",
      execution_id: "exec-audit-failed",
      execution_status: "failed",
      error_code: "ACTION_INVALIDATED",
      resource_type: "host",
      resource_id: host.id,
      resource_name: host.name,
      created_at: "2026-10-27T10:00:00Z",
      updated_at: "2026-10-27T10:00:03Z",
    };
    const make = (
      id: string,
      action: string,
      resourceType: string,
      result: string,
      second: number,
      linked = false,
    ) => ({
      id,
      enterpriseId: enterprise,
      actorUserId: "root",
      actorName: "Audit operator",
      actorType: "enterprise_user",
      action,
      origin: "admin_ui",
      resourceType,
      resourceId: linked ? host.id : id,
      resourceName: linked ? host.name : undefined,
      summary: action,
      result,
      createdAt: `2026-10-27T10:00:0${second}Z`,
      details: linked ? { action_ref: operation.action_ref } : {},
      ...(linked ? { operation } : {}),
      eventHash: "b".repeat(64),
      previousHash: "a".repeat(64),
    });
    db.auditEvents = [
      make(
        "audit-unknown",
        "extension.unregistered",
        "extension_resource",
        "success",
        6,
      ),
      make(
        "audit-tunnel",
        "telemetry.tunnel.disconnect",
        "telemetry_tunnel",
        "success",
        5,
      ),
      make(
        "audit-collection",
        "monitoring.collection.create",
        "collection",
        "success",
        4,
      ),
      make("audit-final", "execution.failed", "host", "failure", 3, true),
      make(
        "audit-confirm",
        "pending_action.confirm",
        "host",
        "success",
        2,
        true,
      ),
      make("audit-preview", "host.update.preview", "host", "success", 1, true),
    ];
    db.executions = [
      {
        execution_id: "exec-audit-failed",
        action_ref: operation.action_ref,
        action_type: "host.update",
        status: "failed",
        one_time_result_state: "unavailable",
        error_code: "ACTION_INVALIDATED",
        created_at: operation.created_at,
        updated_at: operation.updated_at,
        resource_ref: {
          resource_type: "host",
          resource_id: host.id,
          version: 1,
        },
      },
    ];
    localStorage.setItem(key, JSON.stringify(db));
    return host.id as string;
  });
}

for (const english of [false, true])
  for (const theme of ["light", "dark"])
    for (const width of [1280, 1920]) {
      test(`audit operations ${english ? "en" : "zh"} ${theme} ${width}`, async ({
        page,
      }, info) => {
        await page.setViewportSize({ width, height: 1000 });
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
        const hostId = await seed(page);
        await page.goto("/settings/audit");
        await expect(
          page.getByRole("link", { name: /任务记录|Task History/ }),
        ).toHaveCount(0);
        const table = page.getByRole("table");
        await expect(table).toContainText(
          english ? "Disconnect Telemetry tunnel" : "断开遥测隧道",
        );
        await expect(table).toContainText(
          english ? "Create Monitoring collection" : "创建监控采集",
        );
        await expect(table).toContainText(
          english ? "Other operation" : "其他操作",
        );
        await expect(table).not.toContainText("extension.unregistered");
        const confirmation = table
          .getByRole("row")
          .filter({
            hasText: english ? "Confirm pending action" : "确认待执行操作",
          });
        await expect(confirmation).toContainText(english ? "Success" : "成功");
        await expect(confirmation).toContainText(
          english ? "Failed" : "执行失败",
        );
        await confirmation.getByRole("button").click();
        const detail = page.getByRole("dialog");
        await expect(
          detail.getByText("public-web-01", { exact: true }).first(),
        ).toBeVisible();
        await expect(
          detail.getByText(english ? "Operation timeline" : "操作过程", {
            exact: true,
          }),
        ).toBeVisible();
        await expect(detail.locator(".argus-timeline__item")).toHaveCount(3);
        await expect(detail).not.toContainText("act-audit-failed", {
          useInnerText: true,
        });
        await detail.screenshot({
          path: info.outputPath("audit-operation-detail.png"),
          animations: "disabled",
        });
        const auditA11y = await new AxeBuilder({ page }).analyze();
        expect(
          auditA11y.violations.filter(
            (v) => v.impact === "serious" || v.impact === "critical",
          ),
        ).toEqual([]);
        await page.keyboard.press("Escape");
        await page.goto(`/hosts/${hostId}?tab=activity`);
        await expect(
          page.getByRole("tab", {
            name: english ? "Operation progress and results" : "操作进度与结果",
          }),
        ).toHaveAttribute("aria-selected", "true");
        await expect(page.locator(".argus-operation-panel")).toContainText(
          english ? "Failed" : "执行失败",
        );
        await expect(page.locator(".argus-operation-panel")).toContainText(
          "public-web-01",
        );
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= window.innerWidth,
          ),
        ).toBe(true);
        await page.screenshot({
          path: info.outputPath("resource-operation-progress.png"),
          animations: "disabled",
        });
      });
    }
