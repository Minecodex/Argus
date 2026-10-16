import { selectFirstRowAction } from "./helpers/action-menu";
import { test, expect, type Page, type Frame } from "@playwright/test";
import type { AlertConfiguration } from "@argus/api-client";
import AxeBuilder from "@axe-core/playwright";
import { createMfaLogin } from "./helpers/mfa-login";
import { browserApi } from "./helpers/browser-api";
const enabled = process.env.ARGUS_PLAN6_E2E === "1";
const fullAcceptance = process.env.ARGUS_PLAN6_PHASE === "all";
const login = createMfaLogin("enterprise");
async function configs(page: Page): Promise<AlertConfiguration[]> {
  const response = await browserApi(
    page,
    "/api/v1/alerts/configurations?kind=policy",
  );
  expect(response.ok()).toBeTruthy();
  return response.json();
}
async function choose(page: Page, name: string, option: string) {
  await page.getByRole("button", { name, exact: true }).click();
  await page.getByRole("option", { name: option, exact: true }).click();
}
async function signal(page: Page, value: "95" | "60") {
  const response = await page.request.post(
    process.env.ARGUS_PLAN6_SIGNAL_CONTROL_URL!,
    {
      headers: {
        Authorization: `Bearer ${process.env.ARGUS_PLAN6_SIGNAL_CONTROL_TOKEN}`,
      },
      data: { value },
    },
  );
  expect(response.ok()).toBeTruthy();
}
async function fault(page: Page, kind: string, actionRef?: string) {
  const response = await page.request.post(
    process.env.ARGUS_PLAN6_SIGNAL_CONTROL_URL!,
    {
      headers: {
        Authorization: `Bearer ${process.env.ARGUS_PLAN6_SIGNAL_CONTROL_TOKEN}`,
      },
      data: { fault: kind, action_ref: actionRef },
    },
  );
  expect(response.ok()).toBeTruthy();
}
test.describe("Plan6 real portal", () => {
  test.skip(!enabled, "temporary namespace is required");
  // Each case owns its draft/resources and restores shared mutations. With
  // one worker they run in order; an earlier failure must not hide coverage.
  test.describe.configure({ mode: "default", timeout: 120_000 });
  test.beforeAll(() => {
    if (fullAcceptance) {
      for (const key of [
        "ARGUS_PLAN6_CONVERSATION_QUERY",
        "ARGUS_PLAN6_CONVERSATION_CANCEL",
        "ARGUS_PLAN6_UNKNOWN_DELIVERY",
        "ARGUS_PLAN6_MODEL_ID",
      ])
        expect(
          process.env[key],
          `full Plan6 acceptance requires ${key}`,
        ).toBeTruthy();
    }
  });
  test("six live tabs, immutable history and organization entry", async ({
    page,
  }) => {
    await login(
      page,
      "/login",
      process.env.ARGUS_PLAN6_USERNAME ?? "",
      process.env.ARGUS_PLAN6_PASSWORD ?? "",
    );
    await page.goto("/alerts");
    await expect(
      page.getByRole("heading", { name: "告警中心", exact: true }),
    ).toBeVisible();
    await expect(page.getByRole("tab")).toHaveCount(6);
    for (const name of [
      "当前告警",
      "告警历史",
      "通知记录",
      "告警策略",
      "屏蔽与抑制",
      "通知配置",
    ]) {
      await page.getByRole("tab", { name, exact: true }).click();
      await expect(page.getByTestId("alert-center")).toBeVisible();
      await expect(page.getByRole("alert")).toHaveCount(0);
    }
    await page.getByRole("tab", { name: "告警历史", exact: true }).click();
    await page.getByRole("tab", { name: "告警历史", exact: true }).focus();
    await page.keyboard.press("ArrowLeft");
    await expect(
      page.getByRole("tab", { name: "当前告警", exact: true }),
    ).toHaveAttribute("aria-selected", "true");
    await page.goto(
      `/alerts?tab=history&policy_id=${process.env.ARGUS_PLAN6_POLICY_ID ?? ""}&metric=argus_plan6_cpu_percent`,
    );
    await expect(
      page
        .locator("tbody")
        .getByRole("button", { name: "CPU", exact: true })
        .first(),
    ).toBeVisible();
    await page
      .locator("tbody")
      .getByRole("button", { name: "CPU", exact: true })
      .first()
      .click();
    await expect(page.getByRole("dialog")).toBeVisible();
    await expect(
      page
        .getByRole("dialog")
        .getByText("Plan6 real pressure", { exact: true }),
    ).toBeVisible();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "关闭", exact: true })
      .click();
    await page.getByRole("tab", { name: "通知记录", exact: true }).click();
    await expect(
      page.locator("tbody").getByText("已受理", { exact: true }).first(),
    ).toBeVisible();
    const issues = (
      await new AxeBuilder({ page })
        .include('[data-testid="alert-center"]')
        .analyze()
    ).violations.filter((v) =>
      ["serious", "critical"].includes(v.impact ?? ""),
    );
    expect(issues).toEqual([]);
    await page.getByRole("link", { name: "组织与权限", exact: true }).click();
    await page.getByRole("tab", { name: "告警评估", exact: true }).click();
    await expect(
      page.getByRole("button", { name: /后台评估账号/ }),
    ).toBeVisible();
    await page.getByRole("tab", { name: "告警审计", exact: true }).click();
    await expect(
      page.getByRole("heading", { name: "告警审计", exact: true }),
    ).toBeVisible();
  });
  test("real policy draft cancellation and atomic host confirmation", async ({
    page,
  }) => {
    await login(
      page,
      "/login",
      process.env.ARGUS_PLAN6_USERNAME ?? "",
      process.env.ARGUS_PLAN6_PASSWORD ?? "",
    );
    const id = process.env.ARGUS_PLAN6_POLICY_ID ?? "";
    const read = async () => {
      const response = await browserApi(
        page,
        `/api/v1/alerts/configurations/${id}`,
      );
      expect(response.ok()).toBeTruthy();
      return (await response.json()) as {
        version: number;
        spec: { rules: { threshold: number }[] };
      };
    };
    await page.goto("/alerts?tab=policies&query=Plan6%20real%20pressure");
    const before = await read();
    await selectFirstRowAction(page, "编辑");
    const editor = page.getByRole("dialog");
    const threshold = editor
      .getByRole("spinbutton", { name: "阈值", exact: true })
      .first();
    await threshold.fill("86");
    await editor.getByRole("button", { name: "预览变更", exact: true }).click();
    await expect(
      page.getByRole("button", { name: "确认提交", exact: true }),
    ).toBeVisible();
    expect((await read()).spec.rules[0]?.threshold).toBe(
      before.spec.rules[0]?.threshold,
    );
    await page
      .getByRole("dialog", { name: "预览变更", exact: true })
      .getByRole("button", { name: "取消", exact: true })
      .click();
    await expect(threshold).toHaveValue("86");
    expect((await read()).version).toBe(before.version);
    await editor.getByRole("button", { name: "预览变更", exact: true }).click();
    await page.getByRole("button", { name: "确认提交", exact: true }).click();
    await expect
      .poll(async () => (await read()).spec.rules[0]?.threshold)
      .toBe(86);
    expect((await read()).spec.rules).toHaveLength(before.spec.rules.length);
    // Restore the original threshold through the same reviewable UI boundary.
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await selectFirstRowAction(page, "编辑");
    await page
      .getByRole("dialog")
      .getByRole("spinbutton", { name: "阈值", exact: true })
      .first()
      .fill(String(before.spec.rules[0]?.threshold));
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "预览变更", exact: true })
      .click();
    await page.getByRole("button", { name: "确认提交", exact: true }).click();
    await expect
      .poll(async () => (await read()).spec.rules[0]?.threshold)
      .toBe(before.spec.rules[0]?.threshold);
  });

  test("concurrent policy confirmation rejects the old review and retains its draft", async ({
    page,
    context,
  }, testInfo) => {
    await login(
      page,
      "/login",
      process.env.ARGUS_PLAN6_USERNAME ?? "",
      process.env.ARGUS_PLAN6_PASSWORD ?? "",
    );
    const id = process.env.ARGUS_PLAN6_POLICY_ID!;
    const read = async () =>
      (await (
        await browserApi(page, `/api/v1/alerts/configurations/${id}`)
      ).json()) as AlertConfiguration;
    const before = await read();
    const edit = async (target: Page, value: string) => {
      await target.goto("/alerts?tab=policies&query=Plan6%20real%20pressure");
      await selectFirstRowAction(target, "编辑");
      const dialog = target.getByRole("dialog", { name: "编辑", exact: true });
      await dialog
        .getByRole("spinbutton", { name: "阈值", exact: true })
        .first()
        .fill(value);
      await dialog
        .getByRole("button", { name: "预览变更", exact: true })
        .click();
      await expect(
        target.getByRole("dialog", { name: "预览变更", exact: true }),
      ).toBeVisible();
    };
    await edit(page, "87");
    const other = await context.newPage();
    try {
      await edit(other, "88");
      await other
        .getByRole("dialog", { name: "预览变更", exact: true })
        .getByRole("button", { name: "确认提交", exact: true })
        .click();
      await expect
        .poll(async () => (await read()).version)
        .toBe(before.version + 1);
      const committed = await read();
      const denied = page.waitForResponse(
        (r) => r.url().endsWith("/confirm") && r.request().method() === "POST",
      );
      await page
        .getByRole("dialog", { name: "预览变更", exact: true })
        .getByRole("button", { name: "确认提交", exact: true })
        .click();
      expect((await denied).status()).toBe(409);
      expect((await read()).spec).toEqual(committed.spec);
      expect((await read()).version).toBe(committed.version);
      await page
        .getByRole("dialog", { name: "预览变更", exact: true })
        .getByRole("button", { name: "关闭", exact: true })
        .click();
      await expect(
        page
          .getByRole("dialog", { name: "编辑", exact: true })
          .getByRole("spinbutton", { name: "阈值", exact: true })
          .first(),
      ).toHaveValue("87");
      await testInfo.attach("concurrent-policy-reviews", {
        body: JSON.stringify({ before, committed, after: await read() }),
        contentType: "application/json",
      });
    } finally {
      await edit(
        other,
        String(
          (before.spec as { rules: { threshold: number }[] }).rules[0]!
            .threshold,
        ),
      );
      await other
        .getByRole("dialog", { name: "预览变更", exact: true })
        .getByRole("button", { name: "确认提交", exact: true })
        .click();
      await expect.poll(async () => (await read()).spec).toEqual(before.spec);
      await other.close();
    }
  });

  test("real multi-rule create, trial, cancel, lifecycle, handling, silence and authorized export", async ({
    page,
  }, testInfo) => {
    test.setTimeout(240_000);
    await login(
      page,
      "/login",
      process.env.ARGUS_PLAN6_USERNAME ?? "",
      process.env.ARGUS_PLAN6_PASSWORD ?? "",
    );
    const before = await configs(page);
    const base = before.find(
      (p) => p.id === process.env.ARGUS_PLAN6_POLICY_ID,
    )!;
    expect(base).toBeTruthy();
    const baseSpec = base.spec as {
      resources: { source_ids: string[] };
      rules: unknown[];
    };
    const resource = await (
      await browserApi(
        page,
        `/api/v1/enterprise/kubernetes-clusters/${process.env.ARGUS_PLAN6_RESOURCE_ID}`,
      )
    ).json();
    await signal(page, "95");
    await page.goto("/alerts?tab=policies");
    await page
      .getByRole("tabpanel", { name: "告警策略", exact: true })
      .getByRole("button", { name: "创建告警策略", exact: true })
      .click();
    const editor = page.getByRole("dialog");
    await editor
      .getByRole("textbox", { name: /^名称/ })
      .fill("Plan6 browser pressure");
    await choose(page, "生效范围", resource.name);
    await editor
      .getByRole("spinbutton", { name: "检查频率（秒）", exact: true })
      .fill("10");
    await editor
      .getByRole("spinbutton", { name: "触发持续时间（分钟）", exact: true })
      .fill("0");
    await editor
      .getByRole("spinbutton", { name: "恢复持续时间（分钟）", exact: true })
      .fill("0");
    await editor
      .getByRole("textbox", { name: /^指标 \/ PromQL/ })
      .first()
      .fill("argus_plan6_cpu_percent");
    await editor
      .getByRole("button", { name: "添加指标规则", exact: true })
      .click();
    await editor
      .getByRole("textbox", { name: /^指标 \/ PromQL/ })
      .nth(1)
      .fill("argus_plan6_memory_percent");
    await editor
      .getByRole("spinbutton", { name: "阈值", exact: true })
      .nth(1)
      .fill("80");
    await editor
      .getByRole("spinbutton", { name: "恢复阈值", exact: true })
      .nth(1)
      .fill("70");
    await editor.locator("summary").filter({ hasText: "生效范围" }).click();
    await editor
      .getByRole("textbox", { name: "采集来源 UUID（可选）", exact: true })
      .fill(baseSpec.resources.source_ids.join(","));
    await expect(
      editor.getByRole("checkbox", { name: "Plan6 feishu", exact: true }),
    ).toBeChecked();
    const trialResponse = page.waitForResponse(
      (r) =>
        r.url().includes("/alerts/trial") && r.request().method() === "POST",
    );
    await editor.getByRole("button", { name: /试算/ }).click();
    const trial = await (await trialResponse).json();
    expect(trial.health).toBe("normal");
    expect(trial.observations).toHaveLength(2);
    await editor.getByRole("button", { name: "预览变更", exact: true }).click();
    await expect(
      page
        .getByRole("dialog", { name: "预览变更", exact: true })
        .getByText("Plan6 feishu", { exact: true }),
    ).toBeVisible();
    expect(
      (await configs(page)).some((p) => p.name === "Plan6 browser pressure"),
    ).toBeFalsy();
    await page
      .getByRole("dialog", { name: "预览变更", exact: true })
      .getByRole("button", { name: "取消", exact: true })
      .click();
    await expect(editor.getByRole("textbox", { name: /^名称/ })).toHaveValue(
      "Plan6 browser pressure",
    );
    await expect(
      editor.getByRole("textbox", { name: /^指标 \/ PromQL/ }),
    ).toHaveCount(2);
    await expect(
      editor.getByRole("checkbox", { name: "Plan6 feishu", exact: true }),
    ).toBeChecked();
    await editor.getByRole("button", { name: "预览变更", exact: true }).click();
    await page.getByRole("button", { name: "确认提交", exact: true }).click();
    let policy: AlertConfiguration | undefined;
    await expect
      .poll(async () => {
        policy = (await configs(page)).find(
          (p) => p.name === "Plan6 browser pressure",
        );
        return policy?.version;
      })
      .toBe(1);
    expect((policy!.spec as { rules: unknown[] }).rules).toHaveLength(2);
    const policyID = policy!.id;
    const instances = async () =>
      (
        await (
          await browserApi(
            page,
            `/api/v1/alerts/instances?policy_id=${policyID}`,
          )
        ).json()
      ).items as Array<{
        id: string;
        lifecycle: string;
        acknowledged: boolean;
        state: { kind: string; metric: string; notification_state: string };
        owner_id?: string;
      }>;
    await expect
      .poll(
        async () =>
          (await instances()).filter(
            (i) => i.state.kind === "condition" && i.lifecycle === "firing",
          ).length,
        { timeout: 90_000 },
      )
      .toBe(2);
    await page.goto(
      `/alerts?tab=active&policy_id=${policyID}&metric=argus_plan6_cpu_percent`,
    );
    await selectFirstRowAction(page, "详情");
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "确认告警", exact: true })
      .click();
    await expect
      .poll(
        async () =>
          (await instances()).find(
            (i) => i.state.metric === "argus_plan6_cpu_percent",
          )?.acknowledged,
      )
      .toBe(true);
    const members = (
      await (await browserApi(page, "/api/v1/enterprise/users")).json()
    ).items;
    const owner = members.find(
      (u: { username: string }) =>
        u.username === process.env.ARGUS_PLAN6_USERNAME,
    );
    expect(owner).toBeTruthy();
    await choose(page, "负责人", owner.display_name);
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "保存", exact: true })
      .click();
    await expect
      .poll(
        async () =>
          (await instances()).find(
            (i) => i.state.metric === "argus_plan6_cpu_percent",
          )?.owner_id,
      )
      .toBe(owner.id);
    await page
      .getByRole("dialog")
      .getByRole("textbox", { name: /^处理备注/ })
      .fill("Plan6 browser verified handling independent of evaluation");
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "添加备注", exact: true })
      .click();
    await expect(
      page
        .getByRole("dialog")
        .getByText("Plan6 browser verified handling independent of evaluation"),
    ).toBeVisible();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "屏蔽告警", exact: true })
      .click();
    await page
      .getByRole("dialog")
      .getByRole("textbox", { name: /^原因/ })
      .fill("Plan6 browser silence");
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "预览变更", exact: true })
      .click();
    await page.getByRole("button", { name: "确认提交", exact: true }).click();
    await expect
      .poll(
        async () =>
          (await instances()).find(
            (i) => i.state.metric === "argus_plan6_cpu_percent",
          )?.state.notification_state,
        { timeout: 60_000 },
      )
      .toBe("muted");
    await page.goto("/alerts?tab=noise");
    const silenceRow = page
      .locator("tbody tr")
      .filter({ hasText: "Plan6 browser silence" });
    await silenceRow
      .getByRole("button", { name: "解除屏蔽", exact: true })
      .click();
    await page.getByRole("button", { name: "确认提交", exact: true }).click();
    await signal(page, "60");
    await expect
      .poll(
        async () =>
          (
            await (
              await browserApi(
                page,
                `/api/v1/alerts/history?policy_id=${policyID}`,
              )
            ).json()
          ).items.filter(
            (i: { lifecycle: string }) => i.lifecycle === "resolved",
          ).length,
        { timeout: 90_000 },
      )
      .toBe(2);
    await page.goto(`/alerts?tab=history&policy_id=${policyID}`);
    const download = page.waitForEvent("download");
    await page.getByRole("button", { name: "导出记录", exact: true }).click();
    await (
      await download
    ).saveAs(testInfo.outputPath("authorized-alert-history.csv"));
    await expect
      .poll(
        async () =>
          (
            await (
              await browserApi(
                page,
                `/api/v1/alerts/deliveries?policy_id=${policyID}`,
              )
            ).json()
          ).items.some(
            (d: { status: string; message: { event_type: string } }) =>
              d.status === "accepted" && d.message.event_type === "recovery",
          ),
        { timeout: 60_000 },
      )
      .toBe(true);
    const receipts = await (
      await browserApi(page, `/api/v1/alerts/deliveries?policy_id=${policyID}`)
    ).json();
    expect(
      receipts.items.some(
        (d: { status: string; message: { event_type: string } }) =>
          d.status === "accepted" && d.message.event_type === "recovery",
      ),
    ).toBe(true);
    await testInfo.attach("real-browser-business-facts", {
      body: JSON.stringify({
        policy,
        instances: await instances(),
        deliveries: receipts.items,
      }),
      contentType: "application/json",
    });
  });
  test("real model query and cancelled host card remain visible in the existing workspace", async ({
    page,
  }, testInfo) => {
    test.skip(
      !process.env.ARGUS_PLAN6_CONVERSATION_QUERY ||
        !process.env.ARGUS_PLAN6_CONVERSATION_CANCEL,
      "model-stage conversations are outside a browser-only partial run",
    );
    await login(
      page,
      "/login",
      process.env.ARGUS_PLAN6_USERNAME ?? "",
      process.env.ARGUS_PLAN6_PASSWORD ?? "",
    );
    await page.goto(`/?c=${process.env.ARGUS_PLAN6_CONVERSATION_QUERY}`);
    await expect(page.getByTestId("tool-trace").first()).toBeVisible();
    await expect(
      page.getByText(/Plan6|argus_plan6_cpu_percent/).first(),
    ).toBeVisible();
    // A model can also return metric cards. Find the canonical alert result
    // by its business controls rather than assuming presentation order.
    let result: Frame | undefined;
    await expect
      .poll(
        async () => {
          for (const frame of page.frames()) {
            if (await frame.locator(".argus-alert-result #scope").count()) {
              result = frame;
              return true;
            }
          }
          return false;
        },
        { timeout: 60_000 },
      )
      .toBe(true);
    await result!.locator("#scope").click();
    await expect(result!.locator("#scope-facts")).toContainText("最近评估");
    await expect(result!.locator("#completeness")).toContainText(/完整|下一页/);
    await page.screenshot({
      path: testInfo.outputPath("real-model-query-workspace.png"),
      fullPage: true,
    });
    await page.goto(`/?c=${process.env.ARGUS_PLAN6_CONVERSATION_CANCEL}`);
    await expect(page.getByTestId("pending-action-card").first()).toBeVisible();
    await expect(page.getByTestId("pending-action-card").first()).toContainText(
      "已取消",
    );
    await page.screenshot({
      path: testInfo.outputPath("real-model-cancelled-host-card.png"),
      fullPage: true,
    });
  });
  test("ordinary Chinese conversation commits through the workspace host card once", async ({
    page,
  }, testInfo) => {
    test.setTimeout(300_000);
    test.skip(
      !process.env.ARGUS_PLAN6_MODEL_ID,
      "real model is outside a browser-only partial run",
    );
    await login(
      page,
      "/login",
      process.env.ARGUS_PLAN6_USERNAME ?? "",
      process.env.ARGUS_PLAN6_PASSWORD ?? "",
    );
    const before = await configs(page);
    const policy = before.find((item) => item.name === "Plan6 模型双指标策略");
    expect(policy).toBeTruthy();
    const created = await browserApi(page, "/api/v1/conversations", {
      method: "POST",
      data: {
        title: "Plan6 browser workspace confirmation",
        selected_model_id: process.env.ARGUS_PLAN6_MODEL_ID,
      },
      headers: { "Idempotency-Key": `plan6-browser-workspace-${Date.now()}` },
    });
    expect(created.ok()).toBeTruthy();
    const conversation = await created.json();
    const expected = structuredClone(policy!.spec) as {
      rules: Array<{ expression: string; threshold: number }>;
    };
    expected.rules.find(
      (rule) => rule.expression === "argus_plan6_cpu_percent",
    )!.threshold = 92;
    let pendingRef: string | undefined;
    try {
      await page.goto(`/?c=${conversation.id}`);
      const reviewed = page.waitForResponse(async (response) => {
        if (
          response.request().method() !== "GET" ||
          !/\/pending-actions\/[^/]+$/.test(new URL(response.url()).pathname) ||
          !response.ok()
        )
          return false;
        const action = await response.json();
        return action.preview?.id === policy!.id;
      });
      await page
        .getByRole("textbox", { name: "发送", exact: true })
        .fill(
          "请只把已有“Plan6 模型双指标策略”的 CPU 阈值改为92，恢复阈值仍为75，其他所有配置字段（包括描述、规则名称、资源范围、其他规则和通知）保持原样。先给我完整预览，等我确认后提交。",
        );
      await page.getByRole("button", { name: "发送", exact: true }).click();
      const card = page.getByTestId("pending-action-card").last();
      await expect(card).toBeVisible({ timeout: 180_000 });
      await expect(card).toContainText("92");
      // A real Run pauses in waiting_input until host confirmation. Its stop
      // control can remain visible; readiness belongs to the immutable action.
      await expect(
        card.getByRole("button", { name: "确认提交", exact: true }),
      ).toBeEnabled({ timeout: 180_000 });
      const pending = await (await reviewed).json();
      pendingRef = pending.action_ref;
      await testInfo.attach("model-workspace-frozen-preview", {
        body: JSON.stringify({ before: policy, pending }),
        contentType: "application/json",
      });
      expect(pending.preview.spec).toEqual(expected);
      const pendingPolicy = (await configs(page)).find(
        (item) => item.id === policy!.id,
      )!;
      expect(pendingPolicy.version).toBe(policy!.version);
      expect(pendingPolicy.spec).toEqual(policy!.spec);
      let confirmationRequests = 0;
      page.on("request", (request) => {
        if (
          request.method() === "POST" &&
          new URL(request.url()).pathname ===
            `/api/v1/enterprise/pending-actions/${pendingRef}/confirm`
        )
          confirmationRequests++;
      });
      const confirmed = page.waitForResponse(
        (response) =>
          response.request().method() === "POST" &&
          /\/pending-actions\/[^/]+\/confirm$/.test(
            new URL(response.url()).pathname,
          ),
      );
      await card.getByRole("button", { name: "确认提交", exact: true }).click();
      const response = await confirmed;
      expect(response.ok()).toBeTruthy();
      const actionRef = new URL(response.url()).pathname.split("/").at(-2)!;
      await expect
        .poll(
          async () =>
            (await configs(page)).find((item) => item.id === policy!.id)
              ?.version,
          { timeout: 90_000 },
        )
        .toBe(policy!.version + 1);
      const committed = (await configs(page)).find(
        (item) => item.id === policy!.id,
      )!;
      expect(committed.spec).toEqual(expected);
      expect(confirmationRequests).toBe(1);
      await expect(
        card.getByRole("button", { name: "确认提交", exact: true }),
      ).toHaveCount(0);
      const action = await browserApi(
        page,
        `/api/v1/enterprise/pending-actions/${actionRef}`,
      );
      expect(action.ok()).toBeTruthy();
      await testInfo.attach("model-workspace-confirmation", {
        body: JSON.stringify({
          conversation_id: conversation.id,
          action_ref: actionRef,
          confirmation_requests: confirmationRequests,
          before: policy,
          after: committed,
          action: await action.json(),
        }),
        contentType: "application/json",
      });
      await page.screenshot({
        path: testInfo.outputPath("real-model-workspace-confirmed.png"),
        fullPage: true,
      });
    } finally {
      if (pendingRef) {
        const action = await (
          await browserApi(
            page,
            `/api/v1/enterprise/pending-actions/${pendingRef}`,
          )
        ).json();
        if (action.available_actions.includes("cancel")) {
          const cancelled = await browserApi(
            page,
            `/api/v1/enterprise/pending-actions/${pendingRef}/cancel`,
            {
              method: "POST",
              headers: { "Idempotency-Key": crypto.randomUUID() },
            },
          );
          expect(cancelled.ok()).toBeTruthy();
        }
      }
      const current = (await configs(page)).find(
        (item) => item.id === policy!.id,
      )!;
      if (current.version !== policy!.version) {
        const prepared = await browserApi(
          page,
          "/api/v1/alerts/configurations/preview",
          {
            method: "POST",
            data: {
              kind: "policy",
              operation: "update",
              id: policy!.id,
              name: policy!.name,
              spec: policy!.spec,
              expected_version: current.version,
            },
            headers: { "Idempotency-Key": crypto.randomUUID() },
          },
        );
        expect(prepared.ok()).toBeTruthy();
        const cleanup = await prepared.json();
        const restored = await browserApi(
          page,
          `/api/v1/enterprise/pending-actions/${cleanup.action_ref}/confirm`,
          {
            method: "POST",
            headers: { "Idempotency-Key": crypto.randomUUID() },
          },
        );
        expect(restored.ok()).toBeTruthy();
        await expect
          .poll(
            async () =>
              (await configs(page)).find((item) => item.id === policy!.id)
                ?.spec,
            { timeout: 90_000 },
          )
          .toEqual(policy!.spec);
        await testInfo.attach("model-workspace-fixture-restored", {
          body: JSON.stringify({
            action_ref: cleanup.action_ref,
            configuration_id: policy!.id,
          }),
          contentType: "application/json",
        });
      }
    }
  });
  test("notification details and reviewed resend preserve the original Unknown fact", async ({
    page,
  }, testInfo) => {
    test.skip(
      !process.env.ARGUS_PLAN6_UNKNOWN_DELIVERY,
      "notification fault records belong to runtime/all scope",
    );
    await login(
      page,
      "/login",
      process.env.ARGUS_PLAN6_USERNAME ?? "",
      process.env.ARGUS_PLAN6_PASSWORD ?? "",
    );
    const id = process.env.ARGUS_PLAN6_UNKNOWN_DELIVERY!;
    const read = async () =>
      (await (await browserApi(page, `/api/v1/alerts/deliveries/${id}`)).json())
        .delivery;
    const resends = async () =>
      (
        await (
          await browserApi(page, "/api/v1/alerts/deliveries?limit=500")
        ).json()
      ).items.filter(
        (row: { resend_of_id?: string }) => row.resend_of_id === id,
      );
    const original = await read(),
      before = await resends();
    expect(original.status).toBe("unknown");
    const open = async () => {
      await page.goto(`/alerts?tab=deliveries&delivery=${id}`);
      await expect(
        page.getByRole("dialog").locator(".argus-skeleton"),
      ).toHaveCount(0);
      await page
        .getByRole("dialog")
        .getByRole("button", { name: "重新发送", exact: true })
        .click();
      await expect(
        page.getByRole("dialog", { name: "预览变更", exact: true }),
      ).toBeVisible();
    };
    await open();
    expect(await resends()).toHaveLength(before.length);
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "取消", exact: true })
      .click();
    expect(await resends()).toHaveLength(before.length);
    await open();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "确认提交", exact: true })
      .click();
    await expect
      .poll(async () => (await resends()).length)
      .toBe(before.length + 1);
    await expect
      .poll(
        async () =>
          (await resends()).some(
            (row: { id: string; status: string }) =>
              !before.some((old: { id: string }) => old.id === row.id) &&
              row.status === "accepted",
          ),
        { timeout: 60_000 },
      )
      .toBe(true);
    expect(await read()).toEqual(original);
    await testInfo.attach("independent-resend-facts", {
      body: JSON.stringify({ original, before, after: await resends() }),
      contentType: "application/json",
    });
  });
  test("notification pagination, filters and scroll survive details and tab returns", async ({
    page,
  }, testInfo) => {
    await login(
      page,
      "/login",
      process.env.ARGUS_PLAN6_USERNAME ?? "",
      process.env.ARGUS_PLAN6_PASSWORD ?? "",
    );
    const policy = process.env.ARGUS_PLAN6_POLICY_ID!;
    await page.goto(`/alerts?tab=deliveries&policy_id=${policy}`);
    const next = page.getByRole("button", { name: "下一页", exact: true });
    await expect(next).toBeEnabled();
    await next.click();
    await expect(page).toHaveURL(/cursor=/);
    await expect(page.locator(".argus-skeleton")).toHaveCount(0);
    const url = new URL(page.url()),
      cursor = url.searchParams.get("cursor");
    const rows = await page.locator("tbody").innerText();
    await selectFirstRowAction(page, "详情");
    await expect(page.getByRole("dialog")).toBeVisible();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "关闭", exact: true })
      .click();
    expect(new URL(page.url()).searchParams.get("cursor")).toBe(cursor);
    await page.evaluate(() => {
      const main = document.querySelector("main")!;
      main.scrollTo({ top: main.scrollHeight });
    });
    const position = await page
      .locator("main")
      .evaluate((node) => node.scrollTop);
    await page.getByRole("tab", { name: "告警历史", exact: true }).click();
    await page.getByRole("tab", { name: "通知记录", exact: true }).click();
    expect(new URL(page.url()).searchParams.get("policy_id")).toBe(policy);
    expect(new URL(page.url()).searchParams.get("cursor")).toBe(cursor);
    await expect(page.locator("tbody")).toHaveText(rows, {
      useInnerText: true,
    });
    await expect
      .poll(async () => page.locator("main").evaluate((node) => node.scrollTop))
      .toBe(position);
    await page.getByRole("button", { name: "上一页", exact: true }).click();
    await expect
      .poll(() => new URL(page.url()).searchParams.get("cursor"))
      .toBeNull();
    await testInfo.attach("notification-navigation", {
      body: JSON.stringify({ policy, cursor, position, rows }),
      contentType: "application/json",
    });
  });

  test("stale review denial preserves drafts after evaluator revocation and expiry", async ({
    page,
  }, testInfo) => {
    await login(
      page,
      "/login",
      process.env.ARGUS_PLAN6_USERNAME ?? "",
      process.env.ARGUS_PLAN6_PASSWORD ?? "",
    );
    const policyID = process.env.ARGUS_PLAN6_POLICY_ID!;
    const read = async () =>
      await (
        await browserApi(page, `/api/v1/alerts/configurations/${policyID}`)
      ).json();
    const before = await read();
    for (const kind of ["evaluator-disable", "preview-expire"]) {
      await page.goto("/alerts?tab=policies&query=Plan6%20real%20pressure");
      await selectFirstRowAction(page, "编辑");
      const editor = page.getByRole("dialog", { name: "编辑", exact: true });
      const threshold = editor
        .getByRole("spinbutton", { name: "阈值", exact: true })
        .first();
      await threshold.fill("87");
      const prepared = page.waitForResponse(
        (r) =>
          r.url().includes("/alerts/configurations/preview") &&
          r.request().method() === "POST",
      );
      await editor
        .getByRole("button", { name: "预览变更", exact: true })
        .click();
      const pending = await (await prepared).json();
      await fault(page, kind, pending.action_ref);
      try {
        const rejected = page.waitForResponse(
          (r) =>
            r.url().endsWith("/confirm") && r.request().method() === "POST",
        );
        await page
          .getByRole("dialog", { name: "预览变更", exact: true })
          .getByRole("button", { name: "确认提交", exact: true })
          .click();
        const response = await rejected;
        expect(response.status()).toBe(409);
        expect((await response.json()).code).toBe("PENDING_ACTION_INVALIDATED");
        expect((await read()).version).toBe(before.version);
        expect((await read()).spec).toEqual(before.spec);
      } finally {
        if (kind === "evaluator-disable") await fault(page, "evaluator-enable");
      }
      await page
        .getByRole("dialog", { name: "预览变更", exact: true })
        .getByRole("button", { name: "关闭", exact: true })
        .click();
      await expect(threshold).toHaveValue("87");
      await page
        .getByRole("dialog", { name: "编辑", exact: true })
        .getByRole("button", { name: "关闭", exact: true })
        .click();
    }
    await testInfo.attach("stale-draft-policy-unchanged", {
      body: JSON.stringify(await read()),
      contentType: "application/json",
    });
  });
});
