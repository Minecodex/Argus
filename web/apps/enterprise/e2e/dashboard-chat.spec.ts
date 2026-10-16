import { expect, test } from "@playwright/test";
import { openToolResults } from "./helpers/tool-results";
import { randomUUID } from "node:crypto";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const catalog = JSON.parse(
  readFileSync(
    resolve(process.cwd(), "../template-runtime/src/fixtures.generated.json"),
    "utf8",
  ),
);
const binding = catalog.bindings.find(
  (row: { Tool: string }) => row.Tool === "telemetry.dashboard.query",
);
const template = catalog.assets[binding.Hash] as string;
const checksum = binding.Hash as string;

for (const english of [false, true]) {
  test(
    "Chat automatic entry and frozen query parameter presentations (mock UI) " +
      (english ? "English dark" : "Chinese light"),
    async ({ page }, testInfo) => {
      await page.setViewportSize({ width: 1600, height: 1000 });
      await page.addInitScript(
        ({ english }) => {
          if (window !== window.top) return;
          localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
          localStorage.setItem("argus.theme", english ? "dark" : "light");
        },
        { english },
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
      await expect(page).not.toHaveURL(/\/login/);
      await expect(page.getByTestId("chat-context-panel")).toHaveCount(0);
      await expect(
        page.getByRole("button", {
          name: english ? "Manage analysis scope" : "管理分析范围",
        }),
      ).toHaveCount(0);
      const composer = page.locator(".argus-chat-composer");
      const text = composer.getByRole("textbox");
      await text.fill(
        english
          ? "Analyze this host for the last half hour"
          : "分析这台主机最近半小时的指标",
      );
      await composer
        .getByRole("button", { name: english ? "Send" : "发送", exact: true })
        .click();
      await expect(page).toHaveURL(/c=/);
      await expect(text).toHaveValue("", { timeout: 20000 });
      const conversation = new URL(page.url()).searchParams.get("c")!;
      // The fixture supplies immutable server presentation data; it does not
      // simulate model quality or assert that mock telemetry is real.
      await page.evaluate(
        ({ conversation, template, checksum, calls, reads }) => {
          const key = "argus-mock:db-v14";
          const db = JSON.parse(localStorage.getItem(key)!);
          db.messages = db.messages.filter(
            (message: { conversationId: string; role: string }) =>
              message.conversationId !== conversation ||
              message.role === "user",
          );
          const base = {
            name: "Host overview",
            revision_number: 3,
            status: "complete",
            timezone: "Asia/Shanghai",
            to: "2026-10-13T01:30:00Z",
            resources: [
              { id: "host-1", type: "host", name: "argus-test-host" },
            ],
            variables: { env: { all: false, values: ["prod"] } },
            panels: [
              {
                id: "cpu",
                name: "CPU",
                signal: "metrics",
                type: "timeseries",
                unit: "%",
                decimals: 1,
                legend: true,
                layout: { x: 0, y: 0, w: 12, h: 4 },
                targets: [
                  {
                    id: "q",
                    status: "success",
                    data: {
                      resultType: "matrix",
                      result: [
                        {
                          metric: { __name__: "cpu" },
                          values: [
                            [1791853200, "2"],
                            [1791853230, "4"],
                          ],
                        },
                      ],
                    },
                  },
                ],
              },
            ],
          };
          const times = ["2026-10-13T01:00:00Z", "2026-10-13T00:30:00Z"];
          for (const [index, call] of calls.entries()) {
            db.planv5.presentations[call] = {
              conversationId: conversation,
              value: {
                tool_call_id: call,
                runtime: "argus-template/v2",
                read_operations: reads,
                status: "ready",
                template_source: template,
                template_hash: checksum,
                detail_data: { ...base, job_id: call, from: times[index] },
              },
            };
            db.messages.push({
              id: call,
              conversationId: conversation,
              role: "assistant",
              content: "",
              createdAt: new Date(Date.now() + index * 1000).toISOString(),
              toolCalls: [
                {
                  callId: call,
                  toolName: "telemetry.dashboard.query.get",
                  source: "argus",
                  status: "success",
                  startedAt: new Date().toISOString(),
                },
              ],
              presentations: [call],
            });
          }
          localStorage.setItem(key, JSON.stringify(db));
        },
        {
          conversation,
          template,
          checksum,
          calls: [randomUUID(), randomUUID()],
          reads: binding.Reads as string[],
        },
      );
      await page.reload();
      await openToolResults(page);
      const frames = page.locator(".argus-tool-presentation__frame");
      await expect(frames).toHaveCount(2);
      await expect(page.locator(".argus-tool-result__title")).toHaveText([
        english ? "Get dashboard query result" : "仪表盘查询结果",
        english ? "Get dashboard query result" : "仪表盘查询结果",
      ]);
      await expect(frames.first()).toHaveAttribute(
        "title",
        english ? "Get dashboard query result" : "仪表盘查询结果",
      );
      const original = frames.nth(0).contentFrame();
      const corrected = frames.nth(1).contentFrame();
      await expect(
        original.locator(".argus-template-parameters"),
      ).toContainText("argus-test-host");
      await expect(
        original.locator(".argus-template-parameters"),
      ).toContainText("9:00:00");
      await expect(
        corrected.locator(".argus-template-parameters"),
      ).toContainText("8:30:00");
      await expect(
        original.locator(".argus-template-parameters"),
      ).toContainText("Asia/Shanghai");
      await expect(
        original.locator(".argus-dashboard-grid canvas"),
      ).toBeVisible();
      await original
        .locator(".argus-template-parameters > details > summary")
        .click();
      await expect(
        original.locator(".argus-template-parameters"),
      ).toContainText("prod");
      const header = await original
        .locator(".argus-template-parameters")
        .boundingBox();
      const chart = await original
        .locator(".argus-dashboard-grid")
        .boundingBox();
      expect(header!.y + header!.height).toBeLessThanOrEqual(chart!.y);
      const main = await page.locator(".argus-chat__main").boundingBox();
      const shell = await page.locator(".argus-chat").boundingBox();
      expect(main!.width).toBeCloseTo(shell!.width, 0);
      await original
        .locator(".argus-template-parameters > details > summary")
        .click();
      await frames.nth(0).scrollIntoViewIfNeeded();
      await page.screenshot({
        path: testInfo.outputPath("automatic-analysis-parameters.png"),
        fullPage: true,
      });
    },
  );
}
