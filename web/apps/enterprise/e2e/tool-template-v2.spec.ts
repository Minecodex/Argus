import { expect, test, type Page, type TestInfo } from "@playwright/test";
import { readFileSync, writeFileSync } from "node:fs";
import { randomUUID } from "node:crypto";
import { resolve } from "node:path";
import { captureTemplatePanel } from "./helpers/template-panel-capture";
import { openToolResults } from "./helpers/tool-results";

type Binding = {
  Tool: string;
  Entry: string;
  Mode: string;
  Hash: string;
  Reads: string[];
  Data: Record<string, unknown>;
  Expected: string[];
};
const fixtures = JSON.parse(
  readFileSync(
    resolve(process.cwd(), "../template-runtime/src/fixtures.generated.json"),
    "utf8",
  ),
) as {
  bindings: Binding[];
  assets: Record<string, string>;
  cases: Record<string, Record<string, Record<string, unknown>>>;
  scenarios: Record<string, Binding>;
};
const entryFilter = new Set(
  (process.env.ARGUS_TEMPLATE_TEST_ENTRIES ?? "").split(",").filter(Boolean),
);
const scopeLabel = entryFilter.size ? ` [${[...entryFilter].join(",")}]` : "";
const templates = [
  ...new Map(
    fixtures.bindings
      .filter(
        (row) =>
          row.Mode === "template" &&
          (!entryFilter.size || entryFilter.has(row.Entry)),
      )
      .map((row) => [row.Hash, row]),
  ).values(),
];
const sizes = [
  { width: 1280, height: 800 },
  { width: 1600, height: 1000 },
  { width: 1920, height: 1080 },
];

async function session(page: Page, english: boolean, dark: boolean) {
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
    .getByRole("button", { name: english ? "Sign in" : "登录", exact: true })
    .click();
  const composer = page.locator(".argus-chat-composer");
  await composer.getByRole("textbox").fill("template validation");
  await composer
    .getByRole("button", { name: english ? "Send" : "发送", exact: true })
    .click();
  await expect(page).toHaveURL(/c=/);
  await expect(composer.getByRole("textbox")).toHaveValue("", {
    timeout: 20000,
  });
  return new URL(page.url()).searchParams.get("c")!;
}

async function seed(
  page: Page,
  conversation: string,
  binding: Binding,
  data = binding.Data,
  additional?: Record<string, unknown>,
  expanded = true,
  callDetails: { input?: unknown; output?: unknown } = {},
) {
  const call = randomUUID();
  await page.evaluate(
    ({
      conversation,
      binding,
      source,
      data,
      call,
      additional,
      callDetails,
    }) => {
      const key = "argus-mock:db-v14",
        db = JSON.parse(localStorage.getItem(key)!);
      db.messages = db.messages.filter(
        (message: { conversationId: string }) =>
          message.conversationId !== conversation,
      );
      for (const [id, stored] of Object.entries(db.planv5.presentations) as [
        string,
        { conversationId: string },
      ][])
        if (stored.conversationId === conversation)
          delete db.planv5.presentations[id];
      const calls = additional ? [call, crypto.randomUUID()] : [call];
      calls.forEach((id, index) => {
        db.planv5.presentations[id] = {
          conversationId: conversation,
          value: {
            tool_call_id: id,
            runtime: "argus-template/v2",
            status: "ready",
            template_source: source,
            template_hash: binding.Hash,
            read_operations: binding.Reads,
            detail_data: index ? additional : data,
            resource_refs: (index ? additional : data)?._resource_refs ?? [],
          },
        };
        db.messages.push({
          id,
          conversationId: conversation,
          role: "assistant",
          content: "",
          createdAt: new Date(Date.now() + index * 1000).toISOString(),
          toolCalls: [
            {
              callId: id,
              toolName: binding.Tool,
              source: "argus",
              status: "success",
              startedAt: new Date().toISOString(),
              ...callDetails,
            },
          ],
          presentations: [id],
        });
      });
      localStorage.setItem(key, JSON.stringify(db));
    },
    {
      conversation,
      binding,
      source: fixtures.assets[binding.Hash]!,
      data,
      call,
      additional,
      callDetails,
    },
  );
  await page.reload();
  await expect(page.getByTestId("tool-result")).toHaveCount(additional ? 2 : 1);
  if (expanded) await openToolResults(page);
  const card = page.locator(".argus-tool-presentation").first(),
    frame = card.locator("iframe");
  if (expanded) await expect(frame).toBeVisible({ timeout: 25000 });
  return { card, frame, content: frame.contentFrame() };
}

async function geometry(
  page: Page,
  binding: Binding,
  testInfo: TestInfo,
  name: string,
) {
  const frame = page.locator(".argus-tool-presentation iframe").first();
  const content = frame.contentFrame();
  await expect(content.locator("#argus-template-root")).not.toBeEmpty();
  await expect(content.getByRole("alert")).toHaveCount(0);
  await expect(content.locator(".argus-template-spinner")).toHaveCount(0);
  for (const canvas of await content.locator("canvas").all()) {
    const size = await canvas.boundingBox();
    expect(size?.height, `${binding.Entry} chart height`).toBeGreaterThan(120);
  }
  const widths = await content.locator("body").evaluate((body) => ({
    content: body.scrollWidth,
    width: body.clientWidth,
  }));
  expect(widths.content, `${binding.Entry} body overflow`).toBeLessThanOrEqual(
    widths.width + 1,
  );
  await expect
    .poll(
      () =>
        content
          .locator("html")
          .evaluate((root) => root.scrollHeight - root.clientHeight),
      { message: `${binding.Entry} full iframe document` },
    )
    .toBeLessThanOrEqual(1);
  const verticalScrollers = await content
    .locator("body *")
    .evaluateAll((elements) =>
      elements
        .filter((element) => {
          const css = getComputedStyle(element);
          return (
            element.getBoundingClientRect().height > 0 &&
            (["auto", "scroll"].includes(css.overflowY) ||
              element.matches(
                ".argus-template-code, .argus-definition-view pre, .argus-definition-empty, .argus-observation-table, .argus-observation-stats, .argus-observation-logs, .argus-observation-log-body, .argus-trace-view, .argus-dashboard-panel",
              )) &&
            element.scrollHeight > element.clientHeight + 1
          );
        })
        .map((element) => element.className),
    );
  expect(
    verticalScrollers,
    `${binding.Entry} nested vertical scrollbars or clipped content`,
  ).toEqual([]);
  for (const value of await content
    .locator(".argus-observation-stat strong")
    .all()) {
    expect(
      await value.evaluate(
        (element) => element.scrollWidth - element.clientWidth,
      ),
      `${binding.Entry} statistic value and unit fit their column`,
    ).toBeLessThanOrEqual(1);
  }
  const stream = await page.locator(".argus-chat__stream").boundingBox(),
    input = await page.locator(".argus-chat-composer").boundingBox();
  expect(stream!.y + stream!.height).toBeLessThanOrEqual(input!.y + 1);
  const root = await frame.boundingBox(),
    main = await page.locator(".argus-chat__main").boundingBox();
  expect(root!.x).toBeGreaterThanOrEqual(main!.x);
  expect(root!.x + root!.width).toBeLessThanOrEqual(main!.x + main!.width + 1);
  await frame.scrollIntoViewIfNeeded();
  await frame.evaluate((element) =>
    element.scrollIntoView({ block: "start", inline: "nearest" }),
  );
  const captured = await frame.boundingBox();
  const visible = {
    x: captured!.x,
    y: Math.max(captured!.y, stream!.y),
    width: captured!.width,
    height: Math.max(
      0,
      Math.min(captured!.y + captured!.height, stream!.y + stream!.height) -
        Math.max(captured!.y, stream!.y),
    ),
  };
  expect(visible.height).toBeGreaterThan(0);
  await page.screenshot({ path: testInfo.outputPath(name + ".png") });
  return {
    entry: binding.Entry,
    hash: binding.Hash,
    widths,
    frame: captured,
    visible_frame: visible,
    input,
  };
}

test("result tabs explain absent historical input and output", async ({
  page,
}) => {
  const conversation = await session(page, false, false);
  await seed(
    page,
    conversation,
    fixtures.bindings.find((row) => row.Entry === "metric-query")!,
  );
  const card = page.getByTestId("tool-result");
  await card.getByRole("tab", { name: "返回内容", exact: true }).click();
  await expect(
    card.getByText("此调用未记录返回内容。", { exact: true }),
  ).toBeVisible();
  await card.getByRole("tab", { name: "查询参数", exact: true }).click();
  await expect(
    card.getByText("此调用未记录查询参数。", { exact: true }),
  ).toBeVisible();
  await card.getByRole("tab", { name: "视图", exact: true }).click();
  await expect(card.locator("iframe")).toBeVisible();
});

for (const english of [false, true])
  for (const dark of [false, true])
    for (const size of sizes)
      test(`v2 lazy wide disclosure ${english ? "en" : "zh"} ${dark ? "dark" : "light"} ${size.width}`, async ({
        page,
      }, info) => {
        test.setTimeout(60000);
        await page.setViewportSize(size);
        const conversation = await session(page, english, dark);
        const binding = fixtures.bindings.find(
          (row) => row.Entry === "metric-query",
        )!;
        await seed(
          page,
          conversation,
          binding,
          binding.Data,
          binding.Data,
          false,
          {
            input: {
              query: 'db_connections{database="orders"}',
              resource_ids: ["db-prod-01"],
              password: "private-input-must-not-render",
            },
            output: {
              total: 3,
              items: [{ resource: "db-prod-01", value: 15 }],
              secret: "private-output-must-not-render",
            },
          },
        );
        const rows = page.getByTestId("tool-result");
        await expect(rows).toHaveCount(2);
        await expect(
          page.locator(".argus-tool-presentation iframe"),
        ).toHaveCount(0);
        await expect(page.getByTestId("tool-trace")).toHaveCount(0);
        const first = rows.first();
        const collapsed = (await first.boundingBox())!;
        await page.screenshot({ path: info.outputPath("collapsed.png") });
        const trigger = first.locator("header button");
        await trigger.focus();
        await page.keyboard.press("Enter");
        await expect(trigger).toHaveAttribute("aria-expanded", "true");
        const frame = first.locator("iframe");
        await expect(frame).toBeVisible();
        const content = frame.contentFrame();
        await expect(content.locator("canvas")).toHaveCount(1);
        await expect(content.locator(".argus-template-spinner")).toHaveCount(0);
        const originalFrame = await frame.getAttribute("src");
        const filter = content.getByPlaceholder(
          english ? "Search this result…" : "搜索本次结果…",
        );
        await filter.fill("orders");
        await first
          .getByRole("tab", {
            name: english ? "Returned content" : "返回内容",
            exact: true,
          })
          .click();
        await expect(
          first.locator(".argus-chat-result-data:visible pre"),
        ).toContainText('"total": 3');
        await expect(
          first.locator(".argus-chat-result-data:visible pre"),
        ).not.toContainText("private-output-must-not-render");
        await expect(frame).toHaveCount(1);
        await expect(frame).not.toBeVisible();
        await page.screenshot({
          path: info.outputPath("returned-content.png"),
        });
        const parameters = first.getByRole("tab", {
          name: english ? "Query parameters" : "查询参数",
          exact: true,
        });
        await parameters.click();
        await expect(
          first.locator(".argus-chat-result-data:visible pre"),
        ).toContainText("db_connections");
        await expect(
          first.locator(".argus-chat-result-data:visible pre"),
        ).not.toContainText("private-input-must-not-render");
        await page.screenshot({
          path: info.outputPath("query-parameters.png"),
        });
        await parameters.press("ArrowLeft");
        await expect(
          first.getByRole("tab", {
            name: english ? "Returned content" : "返回内容",
            exact: true,
          }),
        ).toBeFocused();
        await first
          .getByRole("tab", { name: english ? "View" : "视图", exact: true })
          .click();
        await expect(frame).toBeVisible();
        await expect(frame).toHaveAttribute("src", originalFrame!);
        await expect(filter).toHaveValue("orders");
        await filter.fill("");
        await expect
          .poll(async () => (await frame.boundingBox())!.height)
          .toBeGreaterThan(480);
        const expanded = (await frame.boundingBox())!;
        expect(expanded.width).toBeGreaterThan(collapsed.width + 40);
        const stream = await page
          .locator(".argus-chat__stream")
          .evaluate((element) => ({
            width: element.clientWidth,
            scrollWidth: element.scrollWidth,
            left: element.getBoundingClientRect().left,
            right: element.getBoundingClientRect().right,
          }));
        expect(stream.scrollWidth).toBeLessThanOrEqual(stream.width + 1);
        expect(expanded.x).toBeGreaterThanOrEqual(stream.left);
        expect(expanded.x + expanded.width).toBeLessThanOrEqual(stream.right);
        const body = await content.locator("body").evaluate((element) => ({
          width: element.clientWidth,
          scrollWidth: element.scrollWidth,
        }));
        expect(body.scrollWidth).toBeLessThanOrEqual(body.width + 1);
        await frame.scrollIntoViewIfNeeded();
        await page.screenshot({ path: info.outputPath("expanded.png") });
        const composer = page.locator(".argus-chat-composer");
        await expect(composer.getByRole("textbox")).toBeVisible();
        const composerBox = (await composer.boundingBox())!;
        const streamBox = (await page
          .locator(".argus-chat__stream")
          .boundingBox())!;
        expect(streamBox.y + streamBox.height).toBeLessThanOrEqual(
          composerBox.y + 1,
        );
        await first.locator("footer button").click();
        await expect(trigger).toBeFocused();
        await expect(
          page.locator(".argus-tool-presentation iframe"),
        ).toHaveCount(0);
        await rows.last().locator("header button").click();
        await expect(rows.last().locator("iframe")).toBeVisible();
        await expect(first.locator("iframe")).toHaveCount(0);
        await page.reload();
        await expect(rows).toHaveCount(2);
        await expect(
          page.locator(".argus-tool-presentation iframe"),
        ).toHaveCount(0);
        writeFileSync(
          info.outputPath("geometry.json"),
          JSON.stringify({ collapsed, expanded, stream, composerBox }, null, 2),
        );
      });

for (const english of [false, true])
  for (const dark of [false, true])
    for (const size of sizes)
      test(`business workspace interactions ${english ? "en" : "zh"} ${dark ? "dark" : "light"} ${size.width}`, async ({
        page,
      }, info) => {
        test.setTimeout(90000);
        await page.setViewportSize(size);
        const conversation = await session(page, english, dark);
        const host = fixtures.bindings.find((row) => row.Tool === "host.list")!;
        const items = Array.from({ length: 120 }, (_, index) => ({
          id: `host-${index}`,
          name: `prod-host-${String(index).padStart(3, "0")}`,
          address: `10.0.0.${index + 1}`,
          connection_status: index % 2 ? "offline" : "connected",
          environment: index % 3 ? "prod" : "staging",
          platform: "linux",
          architecture: "amd64",
        }));
        const { content } = await seed(page, conversation, host, {
          ...host.Data,
          items,
          request_context: { environment: "prod", limit: 120 },
        });
        const search = content.getByRole("searchbox", {
          name: english ? "Search this result…" : "搜索本次结果…",
        });
        await expect(search).toBeVisible();
        await expect(content.locator("tbody>tr")).toHaveCount(25);
        await search.fill("prod-host-119");
        await expect(content.locator("tbody>tr")).toHaveCount(1);
        await expect(content.locator("tbody")).toContainText("prod-host-119");
        await expect(
          content.locator(".argus-template-pagination"),
        ).toContainText(english ? "Returned 120" : "已返回 120");
        await content
          .getByRole("link", { name: "prod-host-119", exact: true })
          .click();
        await expect(content.locator(".argus-result-detail")).toContainText(
          "10.0.0.120",
        );
        await content
          .getByRole("button", {
            name: english ? "Clear filters" : "清空筛选",
            exact: true,
          })
          .click();
        await expect(content.locator("tbody>tr")).toHaveCount(25);
        await content
          .getByRole("button", {
            name: english ? /Connection status/ : /连接状态/,
          })
          .click();
        await content
          .getByRole("option", { name: "offline", exact: true })
          .click();
        await expect(
          content.locator(".argus-template-pagination"),
        ).toContainText(english ? "Matched 60" : "匹配 60");
        await expect(content.locator("tbody>tr").first()).toContainText(
          "prod-host-001",
        );
        await content
          .getByRole("button", {
            name: english ? "Next page" : "下一页",
            exact: true,
          })
          .click();
        await expect(
          content.locator(".argus-template-pagination"),
        ).toContainText("26–50");
        await expect(content.locator("tbody>tr").first()).toContainText(
          "prod-host-051",
        );
        await content
          .getByRole("button", {
            name: english ? "Previous page" : "上一页",
            exact: true,
          })
          .click();
        await expect(content.locator("tbody>tr").first()).toContainText(
          "prod-host-001",
        );
        await search.fill("no-matching-host");
        await expect(content.locator("body")).toContainText(
          english ? "No match in this result" : "本次结果无匹配",
        );
        await expect(
          content.locator(".argus-template-parameters"),
        ).toContainText("prod");
        await page.screenshot({ path: info.outputPath("filtered-list.png") });
        const draft = fixtures.bindings.find(
          (row) => row.Tool === "telemetry.dashboard.draft.get",
        )!;
        const { content: definition } = await seed(page, conversation, draft);
        await expect(definition.locator(".argus-dashboard-tile")).toHaveCount(
          11,
        );
        await expect(definition.locator("canvas")).toHaveCount(0);
        await expect(
          definition.locator(".argus-dashboard-move,.argus-dashboard-resize"),
        ).toHaveCount(0);
        await definition
          .getByRole("button", {
            name: english ? "View configuration" : "查看配置",
            exact: true,
          })
          .first()
          .click();
        const inspector = definition.getByRole("complementary", {
          name: english ? "Panel configuration" : "面板配置",
        });
        await expect(inspector).toBeVisible();
        await expect(
          inspector.locator(".argus-definition-query"),
        ).not.toHaveCount(0);
        await inspector
          .getByRole("tab", { name: english ? "Display" : "展示", exact: true })
          .click();
        await expect(inspector).toContainText("connections");
        await expect(inspector).toContainText("17");
        const inspectorGeometry = await definition
          .locator("body")
          .evaluate((node) => ({
            width: node.clientWidth,
            scrollWidth: node.scrollWidth,
          }));
        expect(inspectorGeometry.scrollWidth).toBeLessThanOrEqual(
          inspectorGeometry.width + 1,
        );
        await page.screenshot({ path: info.outputPath("draft-inspector.png") });
        await inspector
          .getByRole("button", {
            name: english ? "Close panel configuration" : "关闭面板配置",
          })
          .click();
        await definition
          .getByRole("tab", {
            name: english ? "Dashboard configuration" : "仪表盘配置",
          })
          .click();
        await expect(
          definition.locator(".argus-definition-fields"),
        ).not.toBeEmpty();
        await definition
          .getByRole("tab", { name: english ? "Layout preview" : "布局预览" })
          .click();
        await page.screenshot({ path: info.outputPath("draft-layout.png") });
        const dimensions = await definition
          .locator("body")
          .evaluate((node) => ({
            width: node.clientWidth,
            scroll: node.scrollWidth,
          }));
        expect(dimensions.scroll).toBeLessThanOrEqual(dimensions.width + 1);
        expect(await page.locator(".argus-tool-result").count()).toBe(1);
      });

test("compact calls and action previews share one confirmation workspace", async ({
  page,
}, info) => {
  test.setTimeout(60000);
  const conversation = await session(page, false, false);
  const binding = fixtures.bindings.find(
    (row) => row.Tool === "host.collector.upgrade.preview",
  )!;
  await seed(page, conversation, binding, binding.Data, undefined, false);
  await page.evaluate(
    ({ conversation }) => {
      const key = "argus-mock:db-v14",
        db = JSON.parse(localStorage.getItem(key)!);
      const action = db.pendingActions.find(
        (item: { action_type: string; status: string }) =>
          item.action_type === "telemetry.collector.upgrade" &&
          item.status === "awaiting_confirmation",
      );
      if (!action) throw new Error("Missing owned mock confirmation");
      const message = db.messages.find(
        (item: { conversationId: string; presentations?: string[] }) =>
          item.conversationId === conversation && item.presentations?.length,
      );
      const call = message.toolCalls[0],
        id = call.callId;
      call.display = {
        mode: "action",
        action_ref: action.action_ref,
        facts: [{ key: "name", value: action.title }],
      };
      db.planv5.presentations[id].value.detail_data = {
        ...db.planv5.presentations[id].value.detail_data,
        title: action.title,
        summary: action.summary,
        action_type: action.action_type,
        preview: action.preview,
        diff: action.diff,
      };
      message.pendingActionRefs = [action.action_ref];
      db.messages.push({
        id: "action-read",
        conversationId: conversation,
        role: "assistant",
        content: "",
        createdAt: new Date(Date.now() + 1000).toISOString(),
        toolCalls: [
          {
            callId: "read-action",
            toolName: "pending_action.get",
            status: "success",
            startedAt: new Date().toISOString(),
            display: {
              mode: "action",
              action_ref: action.action_ref,
              facts: [],
            },
          },
        ],
      });
      db.messages.push({
        id: "validate",
        conversationId: conversation,
        role: "assistant",
        content: "",
        createdAt: new Date(Date.now() + 2000).toISOString(),
        toolCalls: [
          {
            callId: "validate",
            toolName: "telemetry.dashboard.draft.validate",
            status: "success",
            startedAt: new Date().toISOString(),
            display: {
              mode: "trace",
              facts: [{ key: "validation_issues", value: "1" }],
              issues: [
                {
                  path: "panels[0].targets",
                  severity: "error",
                  message: "Missing query target",
                },
              ],
            },
          },
        ],
      });
      localStorage.setItem(key, JSON.stringify(db));
    },
    { conversation },
  );
  await page.reload();
  const action = page.getByTestId("pending-action-card");
  await expect(action).toHaveCount(1);
  await expect(page.getByTestId("tool-result")).toHaveCount(1);
  await expect(page.locator(".argus-tool-presentation iframe")).toHaveCount(0);
  await expect(
    action.getByRole("button", { name: "确认执行", exact: true }),
  ).toBeVisible();
  await action.locator(".argus-tool-result__header button").click();
  await expect(action.locator("iframe")).toBeVisible();
  await expect(
    action
      .locator("iframe")
      .contentFrame()
      .getByRole("button", { name: /确认执行|取消/ }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: /展开 仪表盘草稿校验 调用详情/ })
    .click();
  await expect(
    page.getByText("Missing query target", { exact: true }),
  ).toBeVisible();
  await page.screenshot({ path: info.outputPath("action-and-validation.png") });
  await page.reload();
  await expect(action).toHaveCount(1);
  await expect(page.getByTestId("tool-result")).toHaveCount(1);
});

test("v2 expanded panels capture visible charts, tall logs and traces", async ({
  page,
}, info) => {
  test.setTimeout(90000);
  await page.setViewportSize(sizes[1]!);
  const conversation = await session(page, false, false);
  const binding = templates.find((row) => row.Entry === "dashboard-result")!;
  // Public sealed data from an owned temporary tenant, including a tall log
  // panel and trace list. This is a renderer regression, not live telemetry.
  const sealed = JSON.parse(
    readFileSync(
      resolve(process.cwd(), "e2e/fixtures/template-dashboard-sealed.json"),
      "utf8",
    ),
  ) as Record<string, unknown>;
  await seed(page, conversation, binding, binding.Data, sealed);
  const frames = page.locator(".argus-tool-presentation iframe");
  await expect(frames).toHaveCount(2);
  const evidence = [];
  for (const [index, frame] of (await frames.all()).entries()) {
    await frame.scrollIntoViewIfNeeded();
    const content = frame.contentFrame();
    await expect(content.locator("h3").first()).not.toBeEmpty();
    const expand = frame
      .locator("..")
      .getByRole("button", { name: "展开详情", exact: true });
    if (await expand.count()) await expand.click();
    for (const [panelIndex, panel] of (
      await content.locator(".argus-dashboard-panel").all()
    ).entries()) {
      const canvas = panel.locator("canvas").first();
      const subject = (await canvas.count())
        ? canvas
        : panel.locator(".argus-template-chart");
      evidence.push(
        await captureTemplatePanel(
          page,
          frame,
          subject,
          info.outputPath(`dashboard-${index}-panel-${panelIndex}.png`),
        ),
      );
      if (index === 1 && panelIndex === 2) {
        const table = panel.locator(".argus-observation-table");
        await table.focus();
        await expect(table).toBeFocused();
        await page.keyboard.press("ArrowRight");
        await expect
          .poll(() => table.evaluate((element) => element.scrollLeft))
          .toBeGreaterThan(0);
        await table.evaluate((element) => {
          element.scrollLeft = 0;
        });
        const dimensions = await table.evaluate((element) => {
          element.scrollTop = element.scrollHeight;
          const box = element.getBoundingClientRect();
          const last = element
            .querySelector("tbody tr:last-child")!
            .getBoundingClientRect();
          return {
            height: element.clientHeight,
            content: element.scrollHeight,
            top: box.top,
            bottom: box.bottom,
            lastTop: last.top,
            lastBottom: last.bottom,
          };
        });
        expect(dimensions.content).toBeLessThanOrEqual(dimensions.height + 1);
        expect(dimensions.lastTop).toBeGreaterThanOrEqual(dimensions.top - 1);
        expect(dimensions.lastBottom).toBeLessThanOrEqual(
          dimensions.bottom + 1,
        );
        await captureTemplatePanel(
          page,
          frame,
          subject,
          info.outputPath("trace-table-last-rows.png"),
        );
      }
    }
  }
  expect(evidence).toHaveLength(14);
  writeFileSync(
    info.outputPath("visible-panel-geometry.json"),
    JSON.stringify(evidence, null, 2),
  );
});

test("v2 single sample is visible at its actual position in the frozen window", async ({
  page,
}, info) => {
  await page.setViewportSize(sizes[1]!);
  const conversation = await session(page, false, false);
  const binding = templates.find((row) => row.Entry === "dashboard-result")!;
  const from = "2026-10-13T01:00:00Z",
    to = "2026-10-13T01:05:00Z";
  const panel = (binding.Data.panels as Array<Record<string, unknown>>)[0]!;
  const { content } = await seed(page, conversation, binding, {
    ...binding.Data,
    from,
    to,
    panels: [
      {
        ...panel,
        legend: false,
        thresholds: [],
        layout: { x: 0, y: 0, w: 12, h: 1 },
        targets: [
          {
            id: "A",
            status: "success",
            result_type: "vector",
            data: {
              result: [
                {
                  metric: { __name__: "single_sample" },
                  value: [Date.parse(to) / 1000, "7"],
                },
              ],
            },
          },
        ],
      },
    ],
  });
  const canvas = content.locator("canvas");
  await expect(canvas).toHaveCount(1);
  const pixels = await canvas.evaluate((element) => {
    const canvas = element as HTMLCanvasElement,
      context = canvas.getContext("2d")!;
    const sample = document.createElement("canvas").getContext("2d")!;
    sample.fillStyle = getComputedStyle(canvas)
      .getPropertyValue("--accent")
      .trim();
    sample.fillRect(0, 0, 1, 1);
    const color = sample.getImageData(0, 0, 1, 1).data,
      data = context.getImageData(0, 0, canvas.width, canvas.height).data;
    let count = 0,
      minX = canvas.width;
    for (let offset = 0; offset < data.length; offset += 4)
      if (
        [0, 1, 2].every(
          (channel) => Math.abs(data[offset + channel]! - color[channel]!) < 6,
        )
      ) {
        count++;
        minX = Math.min(minX, (offset / 4) % canvas.width);
      }
    return { count, minX, width: canvas.width };
  });
  expect(pixels.count).toBeGreaterThan(2);
  expect(pixels.minX).toBeGreaterThan(pixels.width * 0.7);
  const frame = page.locator(".argus-tool-presentation iframe");
  await captureTemplatePanel(
    page,
    frame,
    canvas,
    info.outputPath("single-sample-window.png"),
  );
  writeFileSync(
    info.outputPath("single-sample-pixels.json"),
    JSON.stringify(pixels),
  );
});

for (const english of [false, true])
  for (const dark of [false, true])
    for (const size of sizes) {
      test(`all v2 assets ${english ? "en" : "zh"} ${dark ? "dark" : "light"} ${size.width}${scopeLabel}`, async ({
        page,
      }, info) => {
        test.setTimeout(12 * 60 * 1000);
        await page.setViewportSize(size);
        const failures: string[] = [];
        page.on("pageerror", (error) => failures.push(error.message));
        const conversation = await session(page, english, dark),
          evidence = [];
        for (const [index, binding] of templates.entries()) {
          const { content } = await seed(page, conversation, binding);
          await expect(content.locator("h3").first()).not.toBeEmpty();
          for (const expected of binding.Expected)
            await expect
              .poll(() => content.locator("body").innerText())
              .toContain(expected);
          if (binding.Entry === "metric-query")
            await expect(content.locator("canvas")).toHaveCount(1);
          if (binding.Entry === "dashboard-result")
            await expect(content.locator(".argus-dashboard-panel")).toHaveCount(
              11,
            );
          evidence.push(
            await geometry(page, binding, info, `${index}-${binding.Entry}`),
          );
          const summary = content.locator("summary").first();
          if (await summary.count()) {
            await summary.click();
            await expect(summary).toBeVisible();
          }
        }
        writeFileSync(
          info.outputPath("geometry.json"),
          JSON.stringify(evidence, null, 2),
        );
        expect(failures).toEqual([]);
      });
    }

for (const english of [false, true])
  for (const dark of [false, true]) {
    test(`v2 boundary states ${english ? "en" : "zh"} ${dark ? "dark" : "light"}${scopeLabel}`, async ({
      page,
    }, info) => {
      test.setTimeout(15 * 60 * 1000);
      await page.setViewportSize(sizes[1]!);
      const conversation = await session(page, english, dark);
      const groups = [
        ...new Map(
          templates.map((binding) => [binding.Entry, binding]),
        ).values(),
      ];
      for (const binding of groups)
        for (const variant of [
          "empty",
          "id-only",
          "missing-name",
          "long-name",
          "special-characters",
          "partial",
          "large",
        ]) {
          const { content } = await seed(
            page,
            conversation,
            binding,
            fixtures.cases[binding.Entry]![variant],
          );
          if (variant === "long-name")
            await expect
              .poll(() => content.locator("body").innerText())
              .toContain("Boundary long name");
          if (variant === "special-characters")
            await expect
              .poll(() => content.locator("body").innerText())
              .toContain("<script>alert('xss')</script>");
          await geometry(page, binding, info, `${binding.Entry}-${variant}`);
        }
    });
  }

for (const english of [false, true])
  for (const dark of [false, true])
    for (const size of sizes)
      test(`business contracts ${english ? "en" : "zh"} ${dark ? "dark" : "light"} ${size.width}${scopeLabel}`, async ({
        page,
      }, info) => {
        test.setTimeout(180000);
        await page.setViewportSize(size);
        const conversation = await session(page, english, dark);
        const scenarios = [
          ...Object.entries(fixtures.scenarios),
          ...[
            "telemetry.collector.get",
            "telemetry.collector.list",
            "alert.policy.get",
            "alert.event.get",
          ].map(
            (tool) =>
              [
                tool,
                fixtures.bindings.find((row) => row.Tool === tool)!,
              ] as const,
          ),
        ];
        for (const [name, binding] of scenarios.filter(
          ([, binding]) =>
            binding.Mode === "template" &&
            (!entryFilter.size || entryFilter.has(binding.Entry)),
        )) {
          const { content } = await seed(page, conversation, binding);
          for (const expected of binding.Expected)
            await expect
              .poll(() => content.locator("body").innerText())
              .toContain(expected);
          if (name.startsWith("health-")) {
            if (binding.Tool !== "alert.event.get") {
              await expect(content.locator("tbody")).toContainText(
                english ? "Evaluation error" : "评估失败",
              );
              await content.locator("tbody button").first().click();
            }
            const status = content.locator("[data-alert-health]");
            const health =
              (binding.Data.instance as { health?: string } | undefined)
                ?.health || (name.includes("error-") ? "error" : "unknown");
            const labels: Record<string, string[]> = {
              normal: ["正常", "Normal"],
              error: ["评估失败", "Evaluation error"],
              no_data: ["无数据", "No data"],
              authorization_denied: [
                "评估权限不足",
                "Evaluation access denied",
              ],
              unknown: ["未知", "Unknown"],
            };
            await expect(
              status
                .locator("dt")
                .filter({
                  hasText: english ? /^Evaluation health$/ : /^评估健康$/,
                })
                .locator("+ dd"),
            ).toHaveText(labels[health]![english ? 1 : 0]!);
            if (health !== "normal")
              await expect(status).toContainText(
                english
                  ? "Retained last successful values"
                  : "保留上次成功评估值",
              );
            else
              await expect(status).not.toContainText(
                english
                  ? "Retained last successful values"
                  : "保留上次成功评估值",
              );
          }
          if (name.startsWith("version-") && name !== "version-empty") {
            const index = (
              await content.locator("thead th").allTextContents()
            ).indexOf(english ? "Software version" : "软件版本");
            expect(index).toBeGreaterThan(-1);
            await expect(
              content.locator("tbody tr").first().locator("td").nth(index),
            ).toHaveText(
              name === "version-known"
                ? "v9.8.7"
                : english
                  ? "Unknown"
                  : "未知",
            );
          }
          if (
            name.startsWith("event-kind-") &&
            binding.Tool !== "alert.event.get"
          ) {
            await expect(
              content.getByRole("cell", {
                name: english
                  ? "Not applicable (health event)"
                  : "不适用（健康事件）",
                exact: true,
              }),
            ).toBeVisible();
            await expect(content.locator("tbody")).not.toContainText("— / 0");
          }
          if (
            name.startsWith("log-aggregate-") &&
            name !== "log-aggregate-empty"
          ) {
            await expect(content.locator("tbody")).toContainText("orders-api");
            const index = (
              await content.locator("thead th").allTextContents()
            ).indexOf(english ? "Count" : "计数");
            expect(index).toBeGreaterThan(-1);
            await expect(
              content.locator("tbody tr").first().locator("td").nth(index),
            ).toHaveText(
              name.endsWith("zero") ? "0" : name.endsWith("null") ? "—" : "427",
            );
          }
          if (name.startsWith("trace-root-") || name === "trace-alias-multi") {
            await expect(
              content.locator("[data-trace-result-kind]").first(),
            ).toBeVisible();
            await expect(content.locator("body")).not.toContainText(
              english ? "No data in this range" : "当前范围没有数据",
            );
            if (
              name.endsWith("queryAPMTopology") ||
              name.endsWith("queryAPMRED")
            )
              await expect(content.locator("canvas")).toHaveCount(1);
            if (name.endsWith("queryTrace") || name.endsWith("queryTraceGraph"))
              await expect(content.locator(".argus-trace-view")).toHaveCount(1);
            if (name === "trace-alias-multi")
              await expect(
                content.locator("[data-trace-result-kind]"),
              ).toHaveCount(2);
          }
          if (name.startsWith("apm-limit-")) {
            const state = name.split("-").at(-1)!;
            await expect(content.locator("[data-apm-limited]")).toHaveAttribute(
              "data-apm-limited",
              state,
            );
            await expect(content.locator("[data-apm-limited]")).toContainText(
              state === "true"
                ? english
                  ? "Limited by the business query"
                  : "已受业务限量截断"
                : state === "false"
                  ? english
                    ? "Not truncated by the business limit"
                    : "未被业务限量截断"
                  : english
                    ? "business limit unknown"
                    : "是否受限未知",
            );
            if (state === "true")
              await expect(content.locator("[data-apm-limited]")).toContainText(
                english
                  ? "Card pages only read sealed records"
                  : "卡片翻页仅遍历已封存记录",
              );
          }
          if (
            name.startsWith("apm-red-large-") ||
            name === "dashboard-apm-large"
          ) {
            const chart = content.getByRole("img").filter({ hasText: "" });
            await expect(content.locator("canvas")).toHaveCount(1);
            await expect(chart.first()).toHaveAttribute(
              "aria-label",
              /2026-10-13T01:20:00Z.*2026-10-13T01:29:59Z/,
            );
            await expect(content.locator("body")).toContainText(
              english ? "sample the full time window" : "按全时间窗口采样",
            );
            if (name.startsWith("apm-red-large-")) {
              if (name.endsWith("multi")) {
                const trigger = content
                  .locator(".argus-select__trigger")
                  .first();
                const label = content
                  .locator(".argus-select__value .argus-select__label")
                  .first();
                const size = await label.evaluate((node) => ({
                  visible: node.clientWidth,
                  content: node.scrollWidth,
                }));
                expect(size.content).toBeLessThanOrEqual(size.visible + 1);
                await trigger.click();
                const choice = content.getByRole("option", { name: /P95/ });
                await expect(choice).toBeVisible();
                expect((await choice.boundingBox())!.height).toBeLessThan(100);
                await choice.click();
                await expect(label).toContainText("P95");
              }
              await expect(
                content
                  .getByRole("button", {
                    name: english ? "Next page" : "下一页",
                    exact: true,
                  })
                  .first(),
              ).toBeVisible();
              await content
                .getByRole("button", {
                  name: english ? "Next page" : "下一页",
                  exact: true,
                })
                .first()
                .click();
              await expect(
                content.locator(".argus-template-pagination").first(),
              ).toContainText("26–50");
            }
          }
          if (
            name.startsWith("metric-sampling-") ||
            name.startsWith("dashboard-sampling-")
          ) {
            if (name.endsWith("gap-budget")) {
              await expect(content.locator("body")).toContainText(
                english
                  ? "Missing boundaries exceed"
                  : "缺测边界超过预览点数预算",
              );
              await expect(content.locator("canvas")).toHaveCount(0);
              await expect(content.locator("body")).not.toContainText(
                english ? "No data in this range" : "当前范围没有数据",
              );
            } else {
              await expect(content.locator("canvas")).toHaveCount(1);
              await expect(content.getByRole("img").first()).toHaveAttribute(
                "aria-label",
                english
                  ? /Plotted samples 200, Missing markers 1, Min \/ max 1\.0+ \/ 10000\.0+/
                  : /绘制样本 200, 缺测标记 1, 最小 \/ 最大 1\.0+ \/ 10000\.0+/,
              );
              await expect(content.locator("body")).toContainText(
                english
                  ? "missing boundaries preserved"
                  : "保留端点、极值和缺测边界",
              );
            }
          }
          if (binding.Tool === "alert.event.get") {
            const snapshot = content
              .locator("details")
              .filter({
                has: content.locator("summary", {
                  hasText: english
                    ? "Policy snapshot at trigger"
                    : "触发时策略快照",
                }),
              })
              .first();
            await snapshot.locator(":scope > summary").click();
            await expect(snapshot.locator("tbody").first()).toContainText(
              "host_cpu_usage_percent",
            );
            await expect(snapshot.locator("tbody").first()).toContainText(
              "> 85 %",
            );
            await expect(snapshot.locator("tbody").first()).not.toContainText(
              "> 95 %",
            );
          }
          if (name.startsWith("composite-")) {
            if (binding.Tool !== "alert.event.get")
              await content.locator("tbody button").first().click();
            const members = content.locator("[data-alert-members]");
            await expect(members).toBeVisible();
            await expect(members).toContainText("CPU member");
            await expect(members).toContainText("Memory member");
            await expect(members).toContainText("97.25 %");
            await expect(members).toContainText(
              name.includes("every") ? "4,096.75 MiB" : "1,024.75 MiB",
            );
            await expect(members).toContainText(
              english
                ? name.includes("every")
                  ? "All rules match"
                  : "Any rule matches"
                : name.includes("every")
                  ? "全部规则满足"
                  : "任一规则满足",
            );
            await expect(content.locator("body")).not.toContainText(
              "00000000-0000-0000-0000-000000000000",
            );
          }
          if (name.startsWith("connector-")) {
            const related = content.locator("[data-related-resources]");
            await expect(related).toBeVisible();
            if (name.endsWith("unavailable") || name.endsWith("unassociated")) {
              await expect(related.locator("button")).toHaveCount(0);
              await expect(related).toContainText(
                name.endsWith("unavailable")
                  ? english
                    ? "Not authorized or deleted"
                    : "未授权或已删除"
                  : english
                    ? "No associated host or cluster"
                    : "未关联主机或集群",
              );
            } else await expect(related.locator("button")).toHaveCount(1);
          }
          await geometry(
            page,
            binding,
            info,
            `business-${name.replaceAll(".", "-")}`,
          );
          if (name === "connector-host" || name === "connector-cluster") {
            await content.locator("[data-related-resources] button").click();
            await expect(page).toHaveURL(
              name === "connector-host"
                ? /\/hosts\/10000000-/
                : /\/kubernetes\/10000000-/,
            );
            await page.goto(`/?c=${conversation}`);
          }
        }
      });

test("v2 async placeholder, replacement, failure, paging and historical cards", async ({
  page,
}, info) => {
  test.setTimeout(90000);
  await page.setViewportSize(sizes[1]!);
  const conversation = await session(page, false, false);
  const binding = templates.find((row) => row.Entry === "dashboard-result")!;
  const { card, content } = await seed(page, conversation, binding, {
    ...binding.Data,
    status: "queued",
    _fixture_pending_until: Date.now() + 10000,
  });
  await expect(content.getByRole("status")).toContainText("正在渲染中");
  await expect(content.locator(".argus-template-spinner")).toBeVisible();
  await page.screenshot({ path: info.outputPath("placeholder.png") });
  await expect(content.locator(".argus-dashboard-panel")).toHaveCount(11, {
    timeout: 20000,
  });
  await expect(card).toHaveCount(1);
  await expect(content.locator(".argus-template-spinner")).toHaveCount(0);
  const old = { ...binding.Data, from: "2026-10-13T01:00:00Z" },
    next = { ...binding.Data, from: "2026-10-13T00:30:00Z" };
  await seed(page, conversation, binding, old, next);
  await expect(page.locator(".argus-tool-presentation iframe")).toHaveCount(2);
  await page.reload();
  await expect(page.locator(".argus-tool-presentation iframe")).toHaveCount(0);
  await openToolResults(page);
  await expect(page.locator(".argus-tool-presentation iframe")).toHaveCount(2);
  await seed(page, conversation, binding, {
    ...binding.Data,
    _fixture_read_error: true,
  });
  const failed = page.locator("iframe").last().contentFrame();
  await expect(failed.getByRole("button", { name: "重新加载" })).toBeVisible();
  await expect(failed.locator(".argus-template-spinner")).toHaveCount(0);
  const resource = templates.find((row) => row.Entry === "host-list")!;
  const { content: list } = await seed(
    page,
    conversation,
    resource,
    fixtures.cases[resource.Entry]!.large,
  );
  await expect(list.locator("tbody>tr").filter({ visible: true })).toHaveCount(
    25,
  );
  await list.getByRole("button", { name: "下一页" }).click();
  await expect(list.locator(".argus-template-pagination")).toContainText(
    "26–50",
  );
});

test("v2 frame keeps its card while locale and theme change", async ({
  page,
}, info) => {
  test.setTimeout(60000);
  await page.setViewportSize(sizes[1]!);
  const conversation = await session(page, false, false);
  const binding = templates.find((row) => row.Entry === "metric-query")!;
  const { frame, content } = await seed(page, conversation, binding);
  await expect(content.locator("canvas")).toHaveCount(1);
  const src = await frame.getAttribute("src");
  await page.getByRole("button", { name: "切换到深色模式" }).click();
  await expect(content.locator("html")).toHaveAttribute("data-theme", "dark");
  await page.getByRole("button", { name: "切换语言" }).click();
  await page.getByRole("menuitem", { name: "English", exact: true }).click();
  await expect(content.locator("h3")).toContainText("Metric query");
  await expect(frame).toHaveAttribute("src", src!);
  await geometry(page, binding, info, "theme-locale-switch");
});

for (const english of [false, true])
  for (const dark of [false, true])
    test(`v2 tooltip and keyboard ${english ? "en" : "zh"} ${dark ? "dark" : "light"}`, async ({
      page,
    }, info) => {
      test.setTimeout(60000);
      await page.setViewportSize(sizes[1]!);
      const conversation = await session(page, english, dark);
      const metric = templates.find((row) => row.Entry === "metric-query")!;
      const { card, content } = await seed(page, conversation, metric);
      const expand = card.getByRole("button", {
        name: english ? "Expand details" : "展开详情",
        exact: true,
      });
      if (await expand.count()) await expand.click();
      const chart = content.locator("canvas").first();
      await expect(chart).toBeVisible();
      await chart.scrollIntoViewIfNeeded();
      const pixels = await chart.evaluate((canvas) =>
        (canvas as HTMLCanvasElement).toDataURL(),
      );
      const bounds = await chart.boundingBox();
      await chart.hover({
        position: { x: bounds!.width / 2, y: bounds!.height / 2 },
      });
      await expect
        .poll(
          () =>
            chart.evaluate((canvas) =>
              (canvas as HTMLCanvasElement).toDataURL(),
            ),
          {
            message: "Rich text tooltip must render on the chart",
          },
        )
        .not.toBe(pixels);
      await page.screenshot({ path: info.outputPath("metric-tooltip.png") });
      const log = templates.find((row) => row.Entry === "log-query")!;
      const { content: rows } = await seed(page, conversation, log);
      const detail = rows.locator("tbody tr").first().getByRole("button");
      await detail.focus();
      await detail.press("Enter");
      await expect(detail).toHaveAttribute("aria-expanded", "true");
      await expect(rows.locator("tbody tr").nth(1)).toBeVisible();
      await detail.press("Enter");
      await expect(detail).toHaveAttribute("aria-expanded", "false");
    });

for (const english of [false, true])
  for (const dark of [false, true])
    test(`v2 read failures ${english ? "en" : "zh"} ${dark ? "dark" : "light"}`, async ({
      page,
    }, info) => {
      test.setTimeout(300000);
      await page.setViewportSize(sizes[1]!);
      const conversation = await session(page, english, dark);
      const groups = [
        ...new Map(
          templates.map((binding) => [binding.Entry, binding]),
        ).values(),
      ];
      for (const binding of groups) {
        const { content } = await seed(page, conversation, binding, {
          ...binding.Data,
          _fixture_read_error: true,
        });
        const reload = content
          .getByRole("button", {
            name: english ? "Reload" : "重新加载",
            exact: true,
          })
          .first();
        await expect(reload).toBeVisible();
        await expect(content.locator(".argus-template-spinner")).toHaveCount(0);
        await expect(
          content.locator(".argus-template-note--error"),
        ).toContainText(english ? "Unable to read" : "读取失败");
        await reload.click();
        await expect(reload).toBeVisible();
        await geometry(page, binding, info, binding.Entry + "-read-failure");
      }
    });
