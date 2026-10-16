import { expect, test } from "@playwright/test";
import { openToolResults } from "./helpers/tool-results";
import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { captureTemplatePanel } from "./helpers/template-panel-capture";

const fixtures = JSON.parse(
  readFileSync(
    resolve(process.cwd(), "../template-runtime/src/fixtures.generated.json"),
    "utf8",
  ),
);
const binding = fixtures.bindings.find(
  (row: { Entry: string }) => row.Entry === "dashboard-result",
);
const trace = {
  queryTrace: fixtures.bindings.find(
    (row: { Entry: string }) => row.Entry === "trace-result",
  ).Data.result[0],
};
const row = {
  serviceName: "orders-api",
  instanceId: "orders-01",
  operationName: "POST /orders",
  sampleCount: 10,
  errorCount: 1,
  errorRate: 0.1,
  durationMeanMs: 12,
  durationP95Ms: 20,
  sourceId: "registered-orders-source",
  resourceId: binding.Data.resources[0].id,
};
const apm = (key: string, rows: unknown[]) => ({
  [key]: { basis: "received_entry_spans", status: "available", rows },
});
const extra = [
  {
    type: "logs",
    signal: "logs",
    data: [
      {
        timestamp: "2026-10-13T01:10:00Z",
        service_name: "orders-api",
        severity_text: "WARN",
        body: "Database latency increased",
        resource_id: row.resourceId,
      },
    ],
  },
  {
    type: "trace_list",
    signal: "traces",
    data: {
      queryTraces: {
        total: 1,
        traces: [
          {
            traceId: "trace-001",
            rootService: "orders-api",
            rootOperation: "POST /orders",
            duration: 43,
            status: "error",
            spanCount: 2,
            errorCount: 1,
            startTime: "2026-10-13T01:10:00Z",
          },
        ],
      },
    },
  },
  { type: "trace_detail", signal: "traces", data: trace },
  {
    type: "apm_services",
    signal: "traces",
    data: apm("queryAPMServices", [row]),
  },
  {
    type: "apm_instances",
    signal: "traces",
    data: apm("queryAPMInstances", [row]),
  },
  {
    type: "apm_endpoints",
    signal: "traces",
    data: apm("queryAPMEndpoints", [row]),
  },
  {
    type: "apm_red",
    signal: "traces",
    data: apm("queryAPMRED", [
      { ...row, timestamp: "2026-10-13T01:10:00Z", samplesPerSecond: 2 },
      { ...row, timestamp: "2026-10-13T01:11:00Z", samplesPerSecond: 3 },
    ]),
  },
  {
    type: "apm_topology",
    signal: "traces",
    data: {
      queryAPMTopology: {
        basis: "received_entry_spans",
        status: "available",
        nodes: [
          { id: "orders", serviceName: "orders-api" },
          { id: "database", serviceName: "postgresql" },
        ],
        edges: [
          { sourceNodeId: "orders", targetNodeId: "database", sampleCount: 10 },
        ],
      },
    },
  },
];
const panels = [
  ...binding.Data.panels,
  ...extra.map((item) => ({
    ...item,
    id: item.type,
    name: item.type,
    targets: [
      { id: "A", status: "success", result_type: item.type, data: item.data },
    ],
  })),
].map((panel, index) => ({
  ...panel,
  layout: { x: 0, y: index, w: 12, h: 1 },
}));
const types = panels.map((panel) => panel.type);

for (const english of [false, true])
  for (const dark of [false, true])
    for (const width of [1280, 1600, 1920]) {
      test(`all 19 dashboard chart types ${english ? "en" : "zh"} ${dark ? "dark" : "light"} ${width}`, async ({
        page,
      }, info) => {
        test.setTimeout(120000);
        await page.setViewportSize({
          width,
          height: width === 1280 ? 800 : width === 1600 ? 1000 : 1080,
        });
        await page.addInitScript(
          ({ english, dark }) => {
            if (window !== window.top) return;
            localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
            localStorage.setItem("argus.theme", dark ? "dark" : "light");
          },
          { english, dark },
        );
        await page.goto("/login");
        await page.getByLabel(english ? "Username" : "用户名").fill("root");
        await page.getByLabel(english ? "Password" : "密码").fill("123456");
        await page
          .getByRole("button", {
            name: english ? "Sign in" : "登录",
            exact: true,
          })
          .click();
        const composer = page.locator(".argus-chat-composer");
        await composer
          .getByRole("textbox")
          .fill("Dashboard chart renderer acceptance");
        await composer
          .getByRole("button", { name: english ? "Send" : "发送", exact: true })
          .click();
        await expect(page).toHaveURL(/c=/);
        await expect(composer.getByRole("textbox")).toHaveValue("");
        const conversation = new URL(page.url()).searchParams.get("c")!;
        await page.evaluate(
          ({ binding, source, panels, conversation }) => {
            const key = "argus-mock:db-v14",
              db = JSON.parse(localStorage.getItem(key)!);
            const call = crypto.randomUUID();
            db.messages = db.messages.filter(
              (message: { conversationId: string }) =>
                message.conversationId !== conversation,
            );
            db.planv5.presentations[call] = {
              conversationId: conversation,
              value: {
                tool_call_id: call,
                runtime: "argus-template/v2",
                status: "ready",
                template_source: source,
                template_hash: binding.Hash,
                read_operations: binding.Reads,
                detail_data: { ...binding.Data, panels },
              },
            };
            db.messages.push({
              id: call,
              conversationId: conversation,
              role: "assistant",
              content: "",
              createdAt: new Date().toISOString(),
              presentations: [call],
            });
            localStorage.setItem(key, JSON.stringify(db));
          },
          {
            binding,
            source: fixtures.assets[binding.Hash],
            panels,
            conversation,
          },
        );
        await page.reload();
        await openToolResults(page);
        const frame = page.locator(".argus-tool-presentation iframe"),
          content = frame.contentFrame();
        await expect(frame).toBeVisible();
        await expect(content.locator(".argus-dashboard-panel")).toHaveCount(19);
        const geometry = [];
        for (const [index, panel] of (
          await content.locator(".argus-dashboard-panel").all()
        ).entries()) {
          const canvas = panel.locator("canvas").first();
          const subject = (await canvas.count())
            ? canvas
            : panel.locator(".argus-template-chart");
          await expect(subject).toBeVisible();
          await expect(panel.locator(".argus-observation-empty")).toHaveCount(
            0,
          );
          if (types[index] === "trace_detail") {
            await expect(panel.locator(".argus-trace-view")).toContainText(
              "43.00 ms",
            );
            await expect(panel.locator(".argus-trace-bar")).toHaveCount(2);
          }
          geometry.push({
            type: types[index],
            ...(await captureTemplatePanel(
              page,
              frame,
              subject,
              info.outputPath(`${index}-${types[index]}.png`),
            )),
          });
        }
        expect(new Set(types).size).toBe(19);
        await expect(
          content.locator(".argus-template-note--error"),
        ).toHaveCount(0);
        writeFileSync(
          info.outputPath("all-chart-geometry.json"),
          JSON.stringify(geometry, null, 2),
        );
      });
    }
