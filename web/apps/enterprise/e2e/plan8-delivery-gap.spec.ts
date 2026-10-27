import { expect, test } from "@playwright/test";
import { selectFirstRowAction } from "./helpers/action-menu";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Collector delivery gap ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, testInfo) => {
      test.skip(
        process.env.ARGUS_PLAN6_E2E === "1",
        "isolated mock presentation fixture only",
      );
      await page.addInitScript(
        ({ english, theme }) => {
          localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
          localStorage.setItem("argus.theme", theme);
        },
        { english, theme },
      );
      await page.goto("/login");
      await page
        .getByRole("textbox", {
          name: english ? "Username" : "用户名",
          exact: true,
        })
        .fill("root");
      await page
        .getByRole("textbox", {
          name: english ? "Password" : "密码",
          exact: true,
        })
        .fill("123456");
      await page
        .getByRole("button", {
          name: english ? "Sign in" : "登录",
          exact: true,
        })
        .click();
      await expect(page).not.toHaveURL(/\/login/);
      await page.goto("/alerts");
      await expect(page.getByTestId("alert-center")).toBeVisible();
      // Reading seeds only memory. An isolated mock acknowledgement persists
      // the same visible fixture before adding this presentation-only case.
      await selectFirstRowAction(page, /确认|Ack/);
      await page.evaluate(() => {
        const key = "argus-mock:db-v14";
        const raw = localStorage.getItem(key);
        if (!raw) throw new Error("mock fixture database was not seeded");
        const db = JSON.parse(raw);
        const state = Object.values(db.alerts ?? {}).find(
          (value: any) =>
            Array.isArray(value.instances) && value.instances.length,
        ) as any;
        if (!state) throw new Error("mock alert fixture unavailable");
        const original = state.instances[0],
          policy = state.configurations.find(
            (value: any) => value.id === original.policy_id,
          );
        if (!policy) throw new Error("mock policy fixture unavailable");
        const id = crypto.randomUUID(),
          policyId = crypto.randomUUID(),
          now = new Date().toISOString();
        const report = {
          schema_version: "argus.collector_delivery/v1",
          instance_id: crypto.randomUUID(),
          sequence: 3,
          observed_since: new Date(Date.now() - 360000).toISOString(),
          observed_at: now,
          last_failure_at: new Date(Date.now() - 15000).toISOString(),
          terminal_failures: { log_records: 3, spans: 0, metric_points: 0 },
          receiver_refusals: { log_records: 7, spans: 0, metric_points: 0 },
          enqueue_failures: { log_records: 2, spans: 0, metric_points: 0 },
          retry_limit_seconds: 300,
          attribution: "collector_shared_pipeline_exact_targets_unknown",
        };
        const managed = {
          ...structuredClone(policy),
          id: policyId,
          name: "Collector delivery data gap",
          managed_by: "collector_delivery",
          spec: { ...structuredClone(policy.spec), enabled: false },
        };
        const instance = {
          ...structuredClone(original),
          id,
          policy_id: policyId,
          acknowledged: false,
          lifecycle: "firing",
          health: "error",
          severity: "warning",
          last_evaluated_at: now,
          started_at: now,
          state: {
            ...structuredClone(original.state),
            name: "Collector delivery data gap",
            policy_name: managed.name,
            policy_snapshot: managed.spec,
            kind: "collector_delivery_gap",
            value: undefined,
            metric: "collector.delivery.observed_failures",
            delivery_gap: report,
          },
        };
        state.configurations.unshift(managed);
        state.instances.unshift(instance);
        state.events[id] = [];
        localStorage.setItem(key, JSON.stringify(db));
      });
      await page.reload();
      const title = english
        ? "Collector delivery data gap"
        : "采集发送数据缺口";
      await page
        .getByRole("button", { name: title, exact: true })
        .first()
        .click();
      const panel = page.getByTestId("collector-delivery-gap");
      await expect(panel).toBeVisible();
      await expect(
        panel.getByRole("columnheader", {
          name: english ? "Terminal export failures" : "最终发送失败",
          exact: true,
        }),
      ).toBeVisible();
      await expect(
        panel.getByRole("columnheader", {
          name: english ? "Refused receive attempts" : "接收拒绝尝试",
          exact: true,
        }),
      ).toBeVisible();
      const row = panel
        .getByRole("row")
        .filter({ hasText: english ? "Log records" : "日志记录" });
      await expect(row).toContainText("3");
      await expect(row).toContainText("7");
      await expect(row).toContainText("2");
      await expect(panel).toContainText(
        english ? "exact targets unknown" : "精确目标未知",
      );
      await expect(panel).toContainText("300");
      await expect(
        page.getByRole("heading", {
          name: english ? "Metric trend" : "指标趋势",
          exact: true,
        }),
      ).toHaveCount(0);
      await page.screenshot({
        path: testInfo.outputPath("collector-delivery-gap.png"),
        fullPage: true,
        animations: "disabled",
      });
      const drawer = page.getByRole("dialog").last();
      await drawer
        .getByRole("button", { name: english ? "Close" : "关闭", exact: true })
        .click();
      await page
        .getByRole("tab", {
          name: english ? "Alert policies" : "告警策略",
          exact: true,
        })
        .click();
      await page
        .getByRole("button", { name: title, exact: true })
        .first()
        .click();
      await expect(
        page
          .getByRole("dialog")
          .last()
          .getByRole("button", {
            name: english ? "Edit" : "编辑",
            exact: true,
          }),
      ).toBeDisabled();
      await expect(
        page
          .getByRole("dialog")
          .last()
          .getByRole("button", {
            name: english ? "Duplicate policy" : "复制策略",
            exact: true,
          }),
      ).toBeDisabled();
    });
  }
