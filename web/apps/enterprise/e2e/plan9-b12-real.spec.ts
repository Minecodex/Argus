import { expect, test, type Page } from "@playwright/test";
import { readFileSync, writeFileSync } from "node:fs";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { browserApi } from "./helpers/browser-api";
import { plan7Login } from "./helpers/plan7-login";

const enabled = process.env.ARGUS_PLAN9_B12_REAL === "1";
const out = process.env.ARGUS_PLAN9_B12_OUT ?? "";
const stage = process.env.ARGUS_PLAN9_B12_STAGE ?? "observe";
const mapping = enabled
  ? JSON.parse(readFileSync(process.env.ARGUS_PLAN9_B12_MAP!, "utf8"))
  : {};
const execute = promisify(execFile);
const conversation = mapping.conversation?.id;
const host = mapping.hosts?.direct?.host_id;
test.skip(
  !enabled,
  "Requires the real B12 host/cache, native model and finite quota",
);
test.use({
  ignoreHTTPSErrors: false,
  trace: "off",
  video: "off",
  reducedMotion: "reduce",
});

async function api(page: Page, path: string) {
  const response = await browserApi(page, `/api/v1${path}`);
  expect(response.status()).toBe(200);
  return response.json();
}

async function usage(page: Page) {
  const rows = await api(page, "/enterprise/model-usage");
  return (Array.isArray(rows) ? rows : rows.items).find(
    (row: { model_id: string }) => row.model_id === mapping.model.id,
  );
}

async function send(page: Page, content: string) {
  const composer = page.locator(".argus-chat-composer");
  await composer.getByRole("textbox").fill(content);
  const pending = page.waitForResponse(
    (response) =>
      response.url().endsWith(`/conversations/${conversation}/messages`) &&
      response.request().method() === "POST",
  );
  const preflight = page.waitForResponse(
    (response) =>
      response.url().endsWith(`/conversations/${conversation}/preflight`) &&
      response.request().method() === "POST",
  );
  await composer.getByRole("button", { name: "发送", exact: true }).click();
  const checked = await preflight;
  expect(checked.status(), JSON.stringify(await checked.json())).toBe(200);
  const response = await pending;
  expect(response.status()).toBe(202);
  return (await response.json()).run.run_id as string;
}

async function executions(page: Page, run: string) {
  const value = await api(
    page,
    `/conversations/${conversation}/remote-executions`,
  );
  return value.items.filter(
    (row: { spec: { run_id: string } }) => row.spec.run_id === run,
  );
}

async function waitRun(
  page: Page,
  run: string,
  stopOnAction = false,
  approvedIds = new Set<string>(),
) {
  const deadline = Date.now() + 600_000;
  let printed = 0;
  while (Date.now() < deadline) {
    const billing = await usage(page);
    if (
      billing &&
      (billing.request_count >= 96 ||
        billing.amount >= 0.8 ||
        billing.input_tokens +
          billing.output_tokens +
          billing.estimated_input_tokens +
          billing.estimated_output_tokens >=
          3_200_000)
    ) {
      await browserApi(page, `/api/v1/runs/${run}/cancel`, {
        method: "POST",
        headers: { "Idempotency-Key": `b12-budget-${run}` },
      });
      throw new Error(
        "B12 bounded model budget reached; run cancelled without increasing quota",
      );
    }
    const current = await api(page, `/runs/${run}`);
    if (Date.now() - printed > 15000) {
      console.log(
        `B12 ${stage}: ${current.status}; provider calls=${billing?.request_count ?? 0}`,
      );
      printed = Date.now();
    }
    if (stopOnAction) {
      const pending = (await executions(page, run)).filter(
        (row: {
          id: string;
          status: string;
          confirmation_required?: boolean;
        }) =>
          row.status === "awaiting_confirmation" &&
          row.confirmation_required !== false &&
          !approvedIds.has(row.id),
      );
      if (pending.length) return { current, pending };
    }
    if (
      ["succeeded", "failed", "cancelled", "timed_out"].includes(current.status)
    ) {
      expect(current.status, JSON.stringify(current)).toBe("succeeded");
      return { current, pending: [] };
    }
    await page.waitForTimeout(1000);
  }
  await browserApi(page, `/api/v1/runs/${run}/cancel`, {
    method: "POST",
    headers: { "Idempotency-Key": `b12-timeout-${run}` },
  });
  throw new Error("B12 run exceeded its bounded ten-minute window");
}

const docker = "C:/Program Files/Docker/Docker/resources/bin/docker.exe";
async function physical() {
  const id = mapping.containers.find(
    (row: { service: string }) => row.service === "direct",
  ).id;
  const { stdout: owner } = await execute(docker, [
    "inspect",
    id,
    "--format",
    '{{index .Config.Labels "io.argus.plan9.run"}}',
  ]);
  expect(owner.trim()).toBe(mapping.run_id);
  const { stdout } = await execute(docker, [
    "exec",
    id,
    "sh",
    "-c",
    "systemctl show plan9-b12.service -p ActiveState -p Result -p MainPID; curl -fsS http://127.0.0.1:18084/health || true",
  ]);
  return stdout;
}

function assertOwnedScripts(view: {
  spec: {
    steps: {
      target: { resource_type: string; resource_id: string };
      script: string;
    }[];
  };
}) {
  expect(view.spec.steps.length).toBeGreaterThan(0);
  for (const step of view.spec.steps) {
    expect(step.target.resource_type).toBe("host");
    expect(step.target.resource_id).toBe(host);
    expect(step.script).toMatch(/plan9-b12\.service|127\.0\.0\.1:18084/);
    expect(step.script).not.toMatch(
      /\b(rm|sed|tee|mkfs|mount|umount|reboot|shutdown|passwd|apt|yum|pip|docker|kubectl|iptables|python|perl|node)\b/,
    );
    expect(step.script).not.toMatch(/https:\/\/|\/etc\/|\/root\/|\/home\//);
  }
}

async function actionCard(page: Page, view: { spec: { title: string } }) {
  const card = page
    .getByRole("region", { name: view.spec.title, exact: true })
    .filter({
      has: page.getByRole("button", { name: "确认执行", exact: true }),
    })
    .last();
  await expect(card).toBeVisible({ timeout: 30000 });
  await expect(card).toContainText("B12-Cache-Host");
  return card;
}

async function verifyPublishedReport(page: Page) {
  const published = page
    .locator('a[href*="/deliveries/"][href$="/content"]')
    .last();
  await expect(published).toBeVisible({ timeout: 60000 });
  const href = await published.getAttribute("href");
  expect(href).toBeTruthy();
  const body = await page.evaluate(async (url) => {
    const response = await fetch(url!);
    if (!response.ok) throw new Error("Published report download failed");
    return response.text();
  }, href);
  const expected = JSON.parse(
    readFileSync(`${out}/native-dataset-evidence.json`, "utf8"),
  ).expected;
  expect(body).toContain(String(expected.command_delta));
  expect(body).toContain(String(expected.peak_memory_bytes));
  writeFileSync(`${out}/model-workspace-report.md`, body);
  writeFileSync(
    `${out}/model-published-report-proof.json`,
    JSON.stringify(
      {
        url: href,
        downloaded: true,
        expected_math_verified: true,
        bytes: Buffer.byteLength(body, "utf8"),
      },
      null,
      2,
    ),
  );
}

async function repairFlow(
  page: Page,
  screenshot: (name: string) => Promise<void>,
) {
  await execute("python", ["docs/plan9/runtime/B12/scenario.py", "fault"], {
    cwd: "D:/workspace/go/Argus",
    timeout: 120000,
    env: process.env,
  });
  const fault = JSON.parse(
    readFileSync(`${out}/model-repair-fault.json`, "utf8"),
  );
  const records: Record<string, unknown> = { fault };
  try {
    expect(await physical()).toContain("ActiveState=failed");
    const resumedPath = `${out}/model-pending-cancellation.json`;
    let resumed: { run_id: string } | undefined;
    try {
      resumed = JSON.parse(readFileSync(resumedPath, "utf8"));
    } catch {
      /* First attempt creates its own model proposal. */
    }
    const rejectedRun =
      resumed?.run_id ??
      (await send(
        page,
        "B12-Cache-Host 的 plan9-b12.service 已由测试进程异常退出变为 failed，Redis 和 Collector 不受影响。仅核实本批已有告警，使用 connect 准备一条普通命令执行卡片：对这个 unit reset-failed、start，再 curl http://127.0.0.1:18084/health 验证。只准备并等待卡片确认，本条将测试取消，绝不要提前执行。无需远程诊断取证，不使用 plan.propose，不要提额外问题，不读写任何配置文件，不操作其他资源。",
      ));
    const first = await waitRun(page, rejectedRun, true);
    expect(first.pending.length).toBeGreaterThan(0);
    const rejected = first.pending[0];
    assertOwnedScripts(rejected);
    expect(
      rejected.results.every(
        (row: { status: string }) => row.status === "pending",
      ),
    ).toBe(true);
    const card = await actionCard(page, rejected);
    await screenshot("before-real-cancel");
    await card
      .getByRole("button", { name: /^(取消|取消执行|取消此执行|Cancel)$/ })
      .first()
      .click();
    await expect
      .poll(
        async () =>
          (await executions(page, rejectedRun)).find(
            (row: { id: string }) => row.id === rejected.id,
          )?.status,
        { timeout: 30000 },
      )
      .toBe("cancelled");
    const cancelled = (await executions(page, rejectedRun)).find(
      (row: { id: string }) => row.id === rejected.id,
    );
    expect(
      cancelled.results.some((row: { status: string }) =>
        ["running", "succeeded"].includes(row.status),
      ),
    ).toBe(false);
    const afterCancel = await physical();
    expect(afterCancel).toContain("ActiveState=failed");
    const active = await api(page, `/runs/${rejectedRun}`);
    if (
      !["failed", "cancelled", "succeeded", "timed_out"].includes(active.status)
    ) {
      const stopped = await browserApi(
        page,
        `/api/v1/runs/${rejectedRun}/cancel`,
        {
          method: "POST",
          headers: { "Idempotency-Key": `b12-stop-rejected-${rejectedRun}` },
        },
      );
      expect(stopped.status()).toBe(200);
      await expect
        .poll(async () => (await api(page, `/runs/${rejectedRun}`)).status, {
          timeout: 30000,
        })
        .toBe("cancelled");
    }
    records.cancelled = {
      run_id: rejectedRun,
      request: cancelled,
      physical_after: afterCancel,
      no_native_execution: true,
    };
    writeFileSync(
      `${out}/model-cancellation-proof.json`,
      JSON.stringify(records.cancelled, null, 2),
    );
    await screenshot("real-action-cancelled");
    const repairRun = await send(
      page,
      "上一条卡片已作为取消验证而取消，实际没有执行，服务仍 failed。现在请仅针对同一台 B12-Cache-Host、同一个 plan9-b12.service 重新生成新的普通 connect 执行卡片，reset-failed、start 并 curl 本机18084/health，等待我在新卡片确认。获确认后实际执行，读取结果并重新查询本批平台告警/指标确认恢复，最后中文给出真实证据。不要复用已取消动作，不改配置、不安装包、不触碰其他服务。",
    );
    let approved = 0;
    const approvedIds = new Set<string>();
    while (true) {
      const next = await waitRun(page, repairRun, true, approvedIds);
      if (next.current.status === "succeeded") break;
      for (const view of next.pending) {
        expect(approved).toBeLessThan(3);
        assertOwnedScripts(view);
        const pendingCard = await actionCard(page, view);
        await screenshot(`before-confirm-${approved}`);
        const accepted = page.waitForResponse(
          (response) =>
            response.url().endsWith("/confirm") &&
            response.request().method() === "POST",
        );
        await pendingCard
          .getByRole("button", { name: "确认执行", exact: true })
          .click();
        expect((await accepted).status()).toBe(200);
        approved++;
        approvedIds.add(view.id);
      }
    }
    expect(approved).toBeGreaterThan(0);
    const done = await executions(page, repairRun);
    expect(
      done.some((row: { status: string }) => row.status === "succeeded"),
    ).toBe(true);
    const native = await physical();
    expect(native).toContain("ActiveState=active");
    expect(native).toContain('"healthy": true');
    expect(Date.now()).toBeLessThan(new Date(fault.guard_deadline).getTime());
    const historyPath = `/alerts/history?policy_id=${mapping.alert_policies.service.id}`;
    await expect
      .poll(
        async () =>
          (await api(page, historyPath)).items.some(
            (row: { id: string; lifecycle: string }) =>
              row.id === fault.firing_alert.id && row.lifecycle === "resolved",
          ),
        { timeout: 110000 },
      )
      .toBe(true);
    records.repair = {
      run_id: repairRun,
      approved_cards: approved,
      executions: done,
      native_after: native,
      recovered_before_guard: true,
      usage: await usage(page),
      history: await api(page, historyPath),
    };
    writeFileSync(
      `${out}/model-repair-proof.json`,
      JSON.stringify(records, null, 2),
    );
    await screenshot("real-repair-and-recovery");
  } finally {
    await execute("python", ["docs/plan9/runtime/B12/scenario.py", "recover"], {
      cwd: "D:/workspace/go/Argus",
      timeout: 60000,
      env: process.env,
    });
  }
}

test(`B12 actual model ${stage}`, async ({ page }, info) => {
  test.setTimeout(12 * 60_000);
  page.setDefaultTimeout(25000);
  await page.addInitScript(() => {
    if (!localStorage.getItem("argus.locale"))
      localStorage.setItem("argus.locale", "zh-CN");
    if (!localStorage.getItem("argus.theme"))
      localStorage.setItem("argus.theme", "light");
  });
  await plan7Login(page, "/login", "kakj", process.env.ARGUS_PLAN9_PASSWORD!);
  await page.goto(`/?c=${conversation}`);
  await expect(page.locator(".argus-chat-composer")).toBeVisible();
  const currentConversation = await api(page, `/conversations/${conversation}`);
  expect(currentConversation.selected_model_id).toBe(mapping.model.id);
  expect(currentConversation.access_mode).toBe("request_approval");
  if (stage === "repair_review") {
    const all = (
      await api(page, `/conversations/${conversation}/remote-executions`)
    ).items;
    const cancelledProof = JSON.parse(
      readFileSync(`${out}/model-cancellation-proof.json`, "utf8"),
    );
    const cancelled = all.find(
      (row: { id: string }) => row.id === cancelledProof.request.id,
    );
    expect(cancelled.status).toBe("cancelled");
    expect(
      cancelled.results.some(
        (row: { status: string }) => row.status === "succeeded",
      ),
    ).toBe(false);
    const proof = JSON.parse(
      readFileSync(`${out}/model-confirmed-execution-proof.json`, "utf8"),
    );
    const repaired = all.find((row: { id: string }) => row.id === proof.id);
    expect(repaired.status).toBe("succeeded");
    assertOwnedScripts(repaired);
    const fault = JSON.parse(
      readFileSync(`${out}/model-repair-fault.json`, "utf8"),
    );
    for (const result of repaired.results) {
      expect(result.status).toBe("succeeded");
      expect(result.exit_code).toBe(0);
      expect(result.stdout).toContain("health_http_status=200");
      expect(new Date(result.observed_at).getTime()).toBeLessThan(
        new Date(fault.guard_deadline).getTime(),
      );
    }
    const actualRun = await api(page, `/runs/${repaired.spec.run_id}`);
    expect(actualRun.status).toBe("succeeded");
    const native = await physical();
    expect(native).toContain("ActiveState=active");
    expect(native).toContain('"healthy": true');
    const history = await api(
      page,
      `/alerts/history?policy_id=${mapping.alert_policies.service.id}`,
    );
    expect(
      history.items.some(
        (row: { id: string; lifecycle: string }) =>
          row.id === fault.firing_alert.id && row.lifecycle === "resolved",
      ),
    ).toBe(true);
    await expect(
      page.getByText("执行完成", { exact: true }).first(),
    ).toBeVisible();
    await verifyPublishedReport(page);
    writeFileSync(
      `${out}/model-repair-proof.json`,
      JSON.stringify(
        {
          fault,
          cancelled: cancelledProof,
          repair: {
            run_id: repaired.spec.run_id,
            execution: repaired,
            run: actualRun,
            native_after: native,
            recovered_before_guard: true,
            usage: await usage(page),
            history,
          },
          review_only_no_new_model_messages: true,
        },
        null,
        2,
      ),
    );
    await page.screenshot({
      path: info.outputPath("actual-cancel-confirm-native-recovery.png"),
      fullPage: true,
    });
    for (const width of [1280, 1440, 1920]) {
      await page.setViewportSize({ width, height: 1000 });
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth + 1,
        ),
      ).toBe(true);
      await page.screenshot({
        path: info.outputPath(`review-zh-light-${width}.png`),
      });
    }
    await page.evaluate(() => {
      localStorage.setItem("argus.locale", "en-US");
      localStorage.setItem("argus.theme", "dark");
    });
    await page.reload();
    await expect(page.locator(".argus-chat-composer")).toBeVisible();
    await expect(
      page.getByRole("textbox", { name: "Send", exact: true }),
    ).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
    ).toBe(true);
    await page.screenshot({ path: info.outputPath("review-en-dark-1920.png") });
    return;
  }
  if (stage === "analysis_review") {
    const completed = JSON.parse(
      readFileSync(`${out}/model-analysis.json`, "utf8"),
    );
    expect(completed.run.status).toBe("succeeded");
    expect(
      completed.ledger.items.some(
        (event: {
          event_type: string;
          payload: {
            target_tool_id?: string;
            tool_id?: string;
            status?: string;
          };
        }) =>
          event.event_type === "tool_call_result" &&
          (event.payload.target_tool_id ?? event.payload.tool_id) === "bash" &&
          event.payload.status === "succeeded",
      ),
    ).toBe(true);
    await verifyPublishedReport(page);
    await page.screenshot({
      path: info.outputPath("actual-published-report.png"),
      fullPage: true,
    });
    return;
  }
  if (stage === "repair_flow") {
    await repairFlow(page, async (name) =>
      page
        .screenshot({ path: info.outputPath(`${name}.png`), fullPage: true })
        .then(() => undefined),
    );
    return;
  }
  let prompt: string;
  if (stage === "observe") {
    prompt =
      "这是 B12 真实场景验收。仅查询当前企业的 B12-Cache-Host 和 B12-Redis：先通过平台资源工具核实对象，再查询最近5分钟的主机 CPU/内存和 Redis 当前内存、命令计数、命中/未命中。读取它们已有默认仪表盘的定义和必要面板，解释观测结果并给出真实资源/时间证据。不要扫描全企业，不拉全量进程，不执行远程命令，不修改配置。本轮只读，不要编造未采集的数据。";
  } else if (stage === "metrics") {
    prompt = `B12 补齐尚未完成的平台取数，不重复分析文件。真实对象已核实：B12-Cache-Host (host:${host})，B12-Redis (monitoring_target:${mapping.targets.redis.target_id})。它们已经发布并绑定的仪表盘是 Host ${mapping.dashboards["host.overview"].id}、Redis ${mapping.dashboards.redis.id}。请使用实际平台工具读取这两张仪表盘并执行必要面板，查询最近5分钟 Host CPU/内存、Redis内存/命令/命中率，输出真实数值、单位、资源和时间证据。不能只列目录或定义，也不能拿附件采样代替面板结果。无需再次搜索资源列表，不执行远程命令、不改任何配置，取得足够证据后直接中文总结。`;
  } else if (stage === "analysis") {
    await page.locator('input[type="file"]').setInputFiles({
      name: "b12-native-redis.csv",
      mimeType: "text/csv",
      buffer: readFileSync(`${out}/native-redis-observations.csv`),
    });
    await expect(
      page
        .locator(".argus-chat-chip")
        .filter({ hasText: "b12-native-redis.csv" }),
    ).toBeVisible({ timeout: 120000 });
    prompt =
      "继续 B12 真实验收。上一次只读 Run 在压缩时失败，不能把它算作已完成结论。仅针对 B12-Cache-Host 和 B12-Redis，先读取现有平台指标与默认仪表盘中少量 CPU/内存、Redis 命令/命中面板，报告真实范围/时间/单位，不扫其他资源。附件 b12-native-redis.csv 是此 Redis 的8次真实 INFO，包含监控命令，不能将总命令数当纯业务吞吐。必须实际在 Workspace 执行代码读取附件，计算最后减最初的 total_commands_processed 为 command_delta，used_memory_bytes 最大值为 peak_memory_bytes。写入 /workspace/b12-analysis.md，包含源文件、计算过程、真实观测结论及这两个原始整数字段的 JSON 代码块，并用 workflow.publish_file 发布报告。不要执行任何主机命令，不改配置。充分取证后直接交付，不反复查相同目录。";
  } else {
    throw new Error("Choose the implemented, budgeted B12 stage");
  }
  const run = await send(page, prompt);
  writeFileSync(
    `${out}/model-${stage}-started.json`,
    JSON.stringify({
      run_id: run,
      conversation_id: conversation,
      at: new Date().toISOString(),
    }),
  );
  const result = await waitRun(page, run);
  expect((await executions(page, run)).length).toBe(0);
  const ledger = await api(
    page,
    `/conversations/${conversation}/ledger?limit=100`,
  );
  writeFileSync(
    `${out}/model-${stage}.json`,
    JSON.stringify(
      { run: result.current, usage: await usage(page), ledger },
      null,
      2,
    ),
  );
  await expect(page.getByTestId("tool-trace").first()).toBeVisible();
  if (stage === "analysis") {
    await verifyPublishedReport(page);
  }
  await page.screenshot({
    path: info.outputPath(`actual-model-${stage}.png`),
    fullPage: true,
  });
});
