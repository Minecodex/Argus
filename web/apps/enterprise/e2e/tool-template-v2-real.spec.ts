import { expect, test } from "@playwright/test";
import { createMfaLogin } from "./helpers/mfa-login";
import { browserApi } from "./helpers/browser-api";
import { readFileSync, writeFileSync } from "node:fs";
import { captureTemplatePanel } from "./helpers/template-panel-capture";
import { openToolResults } from "./helpers/tool-results";
const login = createMfaLogin("enterprise");
test("real v2 tool cards recover from stored data and render sealed dashboards", async ({
  page,
}, info) => {
  test.skip(
    process.env.ARGUS_TEMPLATE_E2E !== "1",
    "Requires isolated real tool chain",
  );
  // 92 cards, 17 frozen panels, history reload and a separate change preview.
  // Per-card readiness and metadata request budgets remain unchanged.
  test.setTimeout(420000);
  await page.setViewportSize({ width: 1600, height: 1000 });
  await page.addInitScript(() => {
    if (window !== window.top) return;
    localStorage.setItem("argus.locale", "zh-CN");
    localStorage.setItem("argus.theme", "light");
  });
  await login(
    page,
    "/login",
    process.env.ARGUS_TEMPLATE_USERNAME!,
    process.env.ARGUS_TEMPLATE_PASSWORD!,
  );
  const conversation = process.env.ARGUS_TEMPLATE_CONVERSATION!;
  const metadataRequests = new Map<string, number>();
  const resourceErrors: string[] = [];
  page.on("request", (request) => {
    const prefix = `/api/v1/conversations/${conversation}/tool-presentations/`;
    const path = new URL(request.url()).pathname;
    const call = path.startsWith(prefix) ? path.slice(prefix.length) : "";
    if (request.method() === "GET" && call && !call.includes("/"))
      metadataRequests.set(call, (metadataRequests.get(call) ?? 0) + 1);
  });
  page.on("console", (message) => {
    if (
      message.type() === "error" &&
      message.text().includes("ERR_INSUFFICIENT_RESOURCES")
    )
      resourceErrors.push(message.text());
  });
  await page.goto("/?c=" + conversation);
  const frames = page.locator(".argus-tool-presentation iframe");
  const count = Number(process.env.ARGUS_TEMPLATE_PRESENTATION_COUNT);
  expect(Number.isSafeInteger(count) && count > 0).toBe(true);
  await expect(page.getByTestId("tool-result")).toHaveCount(count, {
    timeout: 30000,
  });
  await openToolResults(page);
  const readEvidence = JSON.parse(
    readFileSync(process.env.ARGUS_TEMPLATE_READ_EVIDENCE!, "utf8"),
  ) as {
    red_fixture: { first_sample_at: string; last_sample_at: string };
    presentations: {
      entry: string;
      collector_last_seen_at: string | null;
      collector_last_seen_present: boolean;
      result_id?: string;
      alert_name?: string;
      alert_health?: string;
      alert_kind?: string;
      log_result_type?: string;
      log_first?: Record<string, unknown>;
      trace_views?: {
        key: string;
        field: string;
        kind: string;
        rows?: Record<string, unknown>[];
        nodes?: Record<string, unknown>[];
        edges?: Record<string, unknown>[];
        spans?: Record<string, unknown>[];
        traceId?: string;
        limited?: boolean;
        chart_preview?: {
          returned_rows: number;
          shown_rows: number;
          series: { points: Record<string, unknown>[] }[];
        };
      }[];
      last_successful_at?: string;
      last_evaluated_at?: string;
      connectors?: { name: string; software_version: string }[];
      related_resources?: {
        id: string;
        name?: string;
        type: string;
        status: string;
      }[];
    }[];
  };
  await expect(frames).toHaveCount(count, { timeout: 30000 });
  const businessViews = new Set<string>();
  for (let index = 0; index < count; index++) {
    const frame = frames.nth(index);
    await frame.scrollIntoViewIfNeeded();
    await expect(frame).toBeVisible();
    const content = frame.contentFrame();
    await expect(content.locator("#argus-template-root")).not.toBeEmpty();
    await expect(
      content.locator(".argus-template-header").first(),
    ).toBeVisible();
    await expect(content.locator(".argus-template-placeholder")).toHaveCount(0);
    await expect(content.locator(".argus-template-note--error")).toHaveCount(0);
    await expect(content.getByRole("alert")).toHaveCount(0);
    const business = content.locator("[data-entry]");
    const entry = await business.getAttribute("data-entry");
    if (entry === "collector-detail") {
      const revision = business
        .locator("dt")
        .filter({ hasText: "期望 / 生效修订" })
        .locator("+ dd");
      await expect(revision).toHaveText(/\d+ \/ \d+/);
      const observed = readEvidence.presentations.find(
        (row) => row.entry === entry,
      )!;
      expect(observed.collector_last_seen_present).toBe(true);
      const lastSeen = business
        .locator("dt")
        .filter({ hasText: "最近采集" })
        .locator("+ dd");
      if (observed.collector_last_seen_at === null)
        await expect(lastSeen).toHaveText("—");
      else {
        await expect(lastSeen).not.toHaveText("—");
        await expect(lastSeen).toContainText(
          String(new Date(observed.collector_last_seen_at).getFullYear()),
        );
      }
      businessViews.add(entry);
    }
    if (entry === "telemetry-catalog") {
      const kind = await business.getAttribute("data-catalog-kind");
      await expect
        .poll(() => business.locator("tbody tr").count())
        .toBeGreaterThan(0);
      await expect(business).not.toContainText("当前范围没有数据");
      businessViews.add(`${entry}:${kind}`);
    }
    if (entry === "alert-policy-detail") {
      await expect(business.locator("tbody").first()).toContainText(
        "argus_m7_e2e_gauge_planv2",
      );
      await expect(business.locator("tbody").first()).toContainText("> 95 %");
      businessViews.add(entry);
    }
    if (entry === "alert-event-detail") {
      let observed: (typeof readEvidence.presentations)[number] | undefined;
      for (const row of readEvidence.presentations.filter(
        (row) => row.entry === entry,
      )) {
        if (
          row.alert_name &&
          (await business
            .getByRole("heading", { name: row.alert_name, exact: true })
            .count())
        ) {
          observed = row;
          break;
        }
      }
      expect(observed).toBeDefined();
      const healthLabels: Record<string, string> = {
        normal: "正常",
        error: "评估失败",
        no_data: "无数据",
        authorization_denied: "评估权限不足",
      };
      const status = business.locator("[data-alert-health]");
      await expect(
        status
          .locator("dt")
          .filter({ hasText: /^评估健康$/ })
          .locator("+ dd"),
      ).toHaveText(healthLabels[observed!.alert_health!]!);
      if (observed!.alert_kind === "health") {
        await expect(status).toContainText("无指标观测值");
        await expect(
          status
            .locator("dt")
            .filter({ hasText: /^最近成功评估$/ })
            .locator("+ dd"),
        ).toHaveText("未知");
        await expect(
          business.locator("dt").filter({ hasText: /^触发值 \/ 阈值$/ }),
        ).toHaveCount(0);
        businessViews.add("alert-kind-health");
      } else {
        await expect(
          status
            .locator("dt")
            .filter({ hasText: /^最近成功评估$/ })
            .locator("+ dd"),
        ).not.toHaveText("未知");
        if (observed!.alert_health !== "normal") {
          expect(
            new Date(observed!.last_successful_at!).getTime(),
          ).toBeLessThan(new Date(observed!.last_evaluated_at!).getTime());
          await expect(status).toContainText("保留上次成功评估值");
        } else await expect(status).not.toContainText("保留上次成功评估值");
        businessViews.add(
          `alert-health:${observed!.alert_name!.includes("recovered") ? "recovered" : observed!.alert_health}`,
        );
        const anyRule =
          (await business
            .getByRole("heading", {
              name: "Template any-rule trigger",
              exact: true,
            })
            .count()) > 0;
        await expect(business).toContainText(
          anyRule ? "97.25 % / 95 %" : "91 % / 85 %",
        );
        await expect(business).toContainText("template maintenance");
        const members = business.locator("[data-alert-members]");
        await expect(members).toBeVisible();
        await expect(members).toContainText("Template memory member");
        await expect(members).toContainText(
          anyRule ? "1,024.75 MiB" : "4,096.75 MiB",
        );
        await expect(members).toContainText(
          anyRule ? "任一规则满足" : "全部规则满足",
        );
        await expect(business).not.toContainText(
          "00000000-0000-0000-0000-000000000000",
        );
        businessViews.add(`composite:${anyRule ? "any" : "every"}`);
        const snapshot = business
          .locator("details")
          .filter({
            has: content.locator("summary", { hasText: "触发时策略快照" }),
          })
          .first();
        await snapshot.locator(":scope > summary").click();
        await expect(snapshot.locator("tbody").first()).toContainText(
          "argus_m7_e2e_gauge_planv2",
        );
        await expect(snapshot.locator("tbody").first()).toContainText(
          anyRule ? "> 95 %" : "> 85 %",
        );
        await expect(snapshot.locator("tbody").first()).not.toContainText(
          anyRule ? "> 85 %" : "> 95 %",
        );
      }
      businessViews.add(entry);
    }
    if (entry === "alert-history") {
      await expect(business).toContainText("Template real policy");
      await business
        .locator("tbody > tr")
        .filter({
          has: content.getByRole("cell", { name: "恢复", exact: true }),
        })
        .first()
        .getByRole("button", { name: "展开", exact: true })
        .click();
      await expect(business.locator("[data-alert-members]")).toContainText(
        "4,096.75 MiB",
      );
      businessViews.add(entry);
    }
    if (entry === "alert-events" || entry === "alert-history") {
      await expect(
        business.getByRole("cell", { name: "不适用（健康事件）", exact: true }),
      ).toHaveCount(2);
    }
    if (entry === "log-query") {
      const type = await business
        .locator("dt")
        .filter({ hasText: /^结果类型$/ })
        .locator("+ dd")
        .innerText();
      if (type === "table" || type === "timeseries") {
        const observed = readEvidence.presentations.find(
          (row) => row.entry === entry && row.log_result_type === type,
        )!;
        expect(observed.log_first).toBeDefined();
        await expect(business.locator("tbody")).toContainText(
          String(observed.log_first!.group_value),
        );
        const index = (
          await business.locator("thead th").allTextContents()
        ).indexOf("计数");
        expect(index).toBeGreaterThan(-1);
        await expect(
          business.locator("tbody tr").first().locator("td").nth(index),
        ).toHaveText(String(observed.log_first!.count));
        businessViews.add("log-aggregate:" + type);
      }
    }
    if (entry === "trace-result") {
      const sections = business.locator("[data-trace-result-kind]");
      await expect(sections.first()).toBeVisible();
      for (const section of await sections.all()) {
        const key = await section.getAttribute("data-trace-result-key"),
          kind = await section.getAttribute("data-trace-result-kind");
        const observed = readEvidence.presentations
          .flatMap((row) => row.trace_views ?? [])
          .find((view) => view.key === key && view.kind === kind)!;
        expect(observed).toBeDefined();
        const rows = observed.rows ?? [];
        if (rows.length) {
          await expect(section.locator("tbody tr").first()).toBeVisible();
          if (rows[0]!.serviceName)
            await expect(section.locator("tbody")).toContainText(
              String(rows[0]!.serviceName),
            );
        } else if (
          kind !== "trace_detail" &&
          kind !== "trace_graph" &&
          kind !== "apm_topology"
        )
          await expect(section).toContainText("当前范围没有数据");
        if (kind === "apm_topology" && (observed.nodes?.length ?? 0) > 0)
          await expect(section.locator("canvas")).toHaveCount(1);
        if (
          (kind === "trace_detail" || kind === "trace_graph") &&
          (observed.spans?.length ?? 0) > 0
        )
          await expect(section.locator(".argus-trace-view")).toHaveCount(1);
        businessViews.add("trace-root:" + observed.field);
        if (key !== observed.field) businessViews.add("trace-alias");
        const limitState = key?.match(/^limit_(true|false|unknown)_/);
        if (limitState) {
          const state = limitState[1]!;
          await expect(section).toHaveAttribute("data-apm-limited", state);
          if (state !== "unknown")
            expect(observed.limited).toBe(state === "true");
          else expect(observed.limited).toBeUndefined();
          if (state === "true")
            await expect(section).toContainText(
              "更多业务结果需要让 AI 继续查询",
            );
          businessViews.add(`apm-limit:${observed.field}:${state}`);
        }
        if (key?.startsWith("template_red_")) {
          expect(observed.chart_preview?.returned_rows).toBe(600);
          const image = section.getByRole("img").first();
          const label = await image.getAttribute("aria-label");
          expect(label).toContain(readEvidence.red_fixture.first_sample_at);
          expect(label).toContain(readEvidence.red_fixture.last_sample_at);
          await expect(section).toContainText("按全时间窗口采样");
          businessViews.add("apm-red-whole-window");
        }
      }
    }
    if (entry === "connector-detail") {
      const id = await business
        .locator("dt")
        .filter({ hasText: /^资源 ID$/ })
        .locator("+ dd")
        .innerText();
      const observed = readEvidence.presentations.find(
        (row) => row.entry === entry && row.result_id === id,
      )!;
      const related = business.locator("[data-related-resources]");
      for (const relation of observed.related_resources ?? []) {
        if (relation.status !== "available") continue;
        await expect(related).toContainText(relation.name!);
        await expect(
          related.getByRole("button", { name: relation.name!, exact: true }),
        ).toBeVisible();
        businessViews.add(`connector:${relation.type}`);
      }
    }
    if (entry === "connector-list") {
      const observed = readEvidence.presentations.find(
        (row) => row.entry === entry,
      )!;
      const index = (
        await business.locator("thead th").allTextContents()
      ).indexOf("软件版本");
      expect(index).toBeGreaterThan(-1);
      for (const row of observed.connectors ?? []) {
        const cells = business
          .locator("tbody > tr")
          .filter({
            has: content.getByRole("cell", { name: row.name, exact: true }),
          })
          .locator("td");
        await expect(cells.nth(index)).toHaveText(
          row.software_version || "未知",
        );
      }
      expect(
        observed.connectors?.some(
          (row) => row.software_version === "fixture-v9.8.7",
        ),
      ).toBe(true);
      expect(observed.connectors?.some((row) => !row.software_version)).toBe(
        true,
      );
      businessViews.add("connector-software-version");
    }
    if (entry === "alert-notifications") {
      await expect(business).toContainText("Template failed notification");
      await expect(business).toContainText("Controlled SMTP failure");
      businessViews.add(entry);
    }
    if (
      entry === "metric-query" &&
      (await business.innerText()).includes("template_sampling_")
    ) {
      const label = await business.innerText();
      const kind = label.includes("template_sampling_gap-budget")
        ? "gap-budget"
        : label.includes("template_sampling_peak")
          ? "peak"
          : "gap";
      if (kind === "gap-budget") {
        await expect(business).toContainText("缺测边界超过预览点数预算");
        await expect(business.locator("canvas")).toHaveCount(0);
      } else {
        await expect(business.getByRole("img").first()).toHaveAttribute(
          "aria-label",
          /绘制样本 200, 缺测标记 1, 最小 \/ 最大 1\.000 \/ 10000\.000/,
        );
      }
      businessViews.add("metric-sampling:" + kind);
    }
    if (await content.locator("#dashboard-view").count()) {
      const plots = content.locator("canvas");
      await expect.poll(() => plots.count()).toBeGreaterThan(0);
      for (const plot of await plots.all()) {
        expect((await plot.boundingBox())!.height).toBeGreaterThan(120);
      }
      for (const empty of await content
        .locator(".argus-observation-empty")
        .all())
        await expect(empty).toContainText("曲线预览不可用");
      const card = frame.locator("..");
      const expand = card.getByRole("button", {
        name: "展开详情",
        exact: true,
      });
      if (await expand.count()) await expand.click();
      const scrollOwners = await content
        .locator(".argus-dashboard-grid")
        .evaluate((grid) => ({
          dashboard: grid.scrollHeight > grid.clientHeight + 1,
          document:
            document.documentElement.scrollHeight >
            document.documentElement.clientHeight + 1,
        }));
      expect(
        Number(scrollOwners.dashboard) + Number(scrollOwners.document),
        "Expanded dashboard must not require two nested vertical scrolls",
      ).toBeLessThanOrEqual(1);
      for (const [panelIndex, panel] of (
        await content.locator(".argus-dashboard-panel").all()
      ).entries()) {
        const panelTitle = await panel.locator("h4").innerText();
        if (panelTitle.startsWith("Real metric sampling ")) {
          const kind = panelTitle.endsWith("gap-budget")
            ? "gap-budget"
            : "peak";
          if (kind === "gap-budget") {
            await expect(panel).toContainText("缺测边界超过预览点数预算");
            await expect(panel.locator("canvas")).toHaveCount(0);
          } else {
            await expect(panel.getByRole("img").first()).toHaveAttribute(
              "aria-label",
              /缺测标记 [1-9]\d*, 最小 \/ 最大 1 \/ 10000/,
            );
          }
          businessViews.add("dashboard-sampling:" + kind);
        }
        if (
          (await panel.locator("h4").innerText()) === "Real APM whole window"
        ) {
          const label = await panel
            .getByRole("img")
            .first()
            .getAttribute("aria-label");
          expect(label).toContain(readEvidence.red_fixture.first_sample_at);
          expect(label).toContain(readEvidence.red_fixture.last_sample_at);
          await expect(panel).toContainText("按全时间窗口采样");
          businessViews.add("dashboard-apm-whole-window");
        }
        const canvas = panel.locator("canvas").first();
        const subject = (await canvas.count())
          ? canvas
          : panel.locator(".argus-template-chart");
        await captureTemplatePanel(
          page,
          frame,
          subject,
          info.outputPath(`real-dashboard-${index}-panel-${panelIndex}.png`),
        );
      }
    }
    const widths = await content.locator("body").evaluate((body) => ({
      width: body.clientWidth,
      scroll: body.scrollWidth,
    }));
    expect(widths.scroll).toBeLessThanOrEqual(widths.width + 1);
    await page.screenshot({
      path: info.outputPath("real-card-" + index + ".png"),
    });
  }
  expect([...businessViews].sort()).toEqual(
    [
      "collector-detail",
      "alert-policy-detail",
      "alert-event-detail",
      "alert-history",
      "alert-notifications",
      "composite:every",
      "composite:any",
      "connector:host",
      "connector:kubernetes_cluster",
      "connector-software-version",
      "alert-kind-health",
      "log-aggregate:table",
      "log-aggregate:timeseries",
      ...[
        "queryBasicTraces",
        "queryBasicTracesByName",
        "queryTraces",
        "queryTrace",
        "queryTraceGraph",
        "queryAPMServices",
        "queryAPMInstances",
        "queryAPMEndpoints",
        "queryAPMRED",
        "queryAPMTopology",
      ].map((root) => "trace-root:" + root),
      "trace-alias",
      "apm-red-whole-window",
      "dashboard-apm-whole-window",
      "metric-sampling:peak",
      "metric-sampling:gap",
      "metric-sampling:gap-budget",
      "dashboard-sampling:peak",
      "dashboard-sampling:gap-budget",
      ...[
        "queryAPMServices",
        "queryAPMInstances",
        "queryAPMEndpoints",
        "queryAPMRED",
        "queryAPMTopology",
      ].flatMap((root) =>
        ["true", "false", "unknown"].map(
          (state) => `apm-limit:${root}:${state}`,
        ),
      ),
      ...[
        "normal",
        "error",
        "no_data",
        "authorization_denied",
        "recovered",
      ].map((health) => `alert-health:${health}`),
    ].sort(),
  );
  writeFileSync(
    info.outputPath("real-business-content.json"),
    JSON.stringify([...businessViews].sort(), null, 2),
  );
  // Initial metadata plus distinct readiness transitions must remain linear.
  expect(metadataRequests.size).toBe(count);
  expect(Math.max(...metadataRequests.values())).toBeLessThanOrEqual(3);
  expect(resourceErrors).toEqual([]);
  writeFileSync(
    info.outputPath("real-startup-requests.json"),
    JSON.stringify(
      {
        passed: true,
        presentations: count,
        metadata_requests: [...metadataRequests.values()].reduce(
          (a, b) => a + b,
          0,
        ),
        maximum_per_card: Math.max(...metadataRequests.values()),
        resource_errors: resourceErrors.length,
      },
      null,
      2,
    ),
  );
  const events = [];
  let cursor = 0;
  for (let batch = 0; batch < 20; batch++) {
    const ledger = await (
      await browserApi(
        page,
        "/api/v1/conversations/" +
          conversation +
          "/ledger?limit=200&cursor=" +
          cursor,
      )
    ).json();
    events.push(...ledger.items);
    if (ledger.items.length < 200) break;
    cursor = ledger.items.at(-1).sequence;
    if (batch === 19)
      throw new Error("Conversation ledger exceeded the acceptance bound");
  }
  const ready = events.filter(
    (event: { event_type: string; payload: { status: string } }) =>
      event.event_type === "tool_presentation" &&
      event.payload.status === "data_ready",
  );
  expect(ready).toHaveLength(2);
  // Read each exact persisted notification through the authenticated SSE
  // endpoint, including Last-Event-ID recovery after the Run has ended.
  const streamed = await page.evaluate(
    async ({ conversation, ready }) => {
      const events: unknown[] = [];
      for (const event of ready) {
        const abort = new AbortController();
        const timeout = setTimeout(() => abort.abort(), 15000);
        try {
          const response = await fetch(
            `/api/v1/conversations/${conversation}/events`,
            {
              headers: {
                Accept: "text/event-stream",
                "Last-Event-ID": String(event.sequence - 1),
              },
              signal: abort.signal,
            },
          );
          if (!response.ok || !response.body)
            throw new Error("SSE read failed");
          const reader = response.body.getReader();
          const decoder = new TextDecoder();
          let buffer = "";
          while (!buffer.includes("\n\n")) {
            const part = await reader.read();
            if (part.done) throw new Error("SSE ended before the ready event");
            buffer += decoder.decode(part.value, { stream: true });
          }
          const frame = buffer.split("\n\n")[0]!;
          const data = frame
            .split("\n")
            .find((line) => line.startsWith("data: "));
          if (!data) throw new Error("SSE frame has no data");
          events.push(JSON.parse(data.slice(6)));
          await reader.cancel();
        } finally {
          clearTimeout(timeout);
          abort.abort();
        }
      }
      return events;
    },
    { conversation, ready },
  );
  expect(streamed.map((event: any) => event.data.payload.tool_call_id)).toEqual(
    ready.map((event: any) => event.payload.tool_call_id),
  );
  expect(streamed.map((event: any) => event.data.payload.status)).toEqual([
    "data_ready",
    "data_ready",
  ]);
  writeFileSync(
    info.outputPath("real-sse-ready.json"),
    JSON.stringify(streamed, null, 2),
  );
  await page.reload();
  await expect(page.getByTestId("tool-result")).toHaveCount(count, {
    timeout: 30000,
  });
  await openToolResults(page);
  await expect(frames).toHaveCount(count);
  await expect(page.locator(".argus-chat-composer")).toBeVisible();
  await page.goto("/?c=" + process.env.ARGUS_TEMPLATE_PREVIEW_CONVERSATION!);
  await openToolResults(page);
  const preview = page.locator(".argus-tool-presentation iframe").first();
  await expect(preview).toBeVisible();
  await expect(
    preview.contentFrame().locator("#argus-template-root"),
  ).toContainText("Tool template owned host preview");
  await expect(preview.contentFrame().locator("table tbody")).toContainText(
    "m3-direct-host",
  );
  await expect(
    preview.contentFrame().getByRole("button", { name: /确认|取消/ }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "确认执行", exact: true }),
  ).toBeVisible();
  await page.screenshot({ path: info.outputPath("real-change-preview.png") });
});
