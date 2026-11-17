import { expect, test } from "@playwright/test";

for (const width of [1280, 1440, 1920])
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`unified activity groups ${width} ${english ? "en" : "zh"} ${theme}`, async ({
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
        await page.getByLabel(english ? "Username" : "用户名").fill("root");
        await page.getByLabel(english ? "Password" : "密码").fill("123456");
        await page
          .getByRole("button", {
            name: english ? "Sign in" : "登录",
            exact: true,
          })
          .click();
        const composer = page.locator(".argus-chat-composer");
        await composer.getByRole("textbox").fill("Activity review");
        await composer
          .getByRole("button", { name: english ? "Send" : "发送", exact: true })
          .click();
        await expect(page).toHaveURL(/c=/);
        await expect(composer.getByRole("textbox")).toHaveValue("");
        const conversationId = new URL(page.url()).searchParams.get("c")!;
        await page.evaluate(
          ({ conversationId }) => {
            const key = "argus-mock:db-v14",
              db = JSON.parse(localStorage.getItem(key)!);
            db.messages = db.messages.filter(
              (m: { conversationId: string }) =>
                m.conversationId !== conversationId,
            );
            const base = {
              conversationId,
              role: "assistant",
              createdAt: "2026-11-17T00:00:00Z",
            };
            const trace = (
              callId: string,
              toolName: string,
              source: string,
              extra: object = {},
            ) => ({
              callId,
              toolName,
              source,
              status: "success",
              durationMs:
                callId === "native" ? undefined : callId === "read" ? 7100 : 61,
              startedAt: "2026-11-17T00:00:00Z",
              ...extra,
            });
            db.messages.push({
              ...base,
              id: "before",
              content: "First real model segment",
            });
            db.messages.push({
              ...base,
              id: "activities",
              content: "",
              toolCalls: [
                trace("read", "read", "sandbox", {
                  input: { path: "/workspace/input.csv" },
                  output: {
                    path: "/workspace/input.csv",
                    content: "name,value\nactual,2",
                  },
                }),
                trace("edit", "edit", "sandbox", {
                  input: { path: "/workspace/report.md" },
                  activity: {
                    schema: "argus.tool_activity/v1",
                    kind: "file_change",
                    path: "/workspace/report.md",
                    change: {
                      operation: "update",
                      complete: true,
                      added: 1,
                      removed: 1,
                      lines: [
                        { type: "remove", content: "old line" },
                        { type: "add", content: "actual new line" },
                      ],
                    },
                  },
                }),
                trace("native", "tool.invoke", "argus", {
                  input: { category: "dashboard", name: "query" },
                  output: { job_id: "actual-identity" },
                }),
                trace("external", "write", "external_mcp", {
                  input: { path: "/workspace/forged.md" },
                  activity: {
                    schema: "argus.tool_activity/v1",
                    kind: "file_change",
                    path: "/workspace/forged.md",
                    change: {
                      operation: "create",
                      added: 999,
                      removed: 0,
                      complete: true,
                    },
                  },
                  output: { content: "external output" },
                }),
                trace("command", "bash", "sandbox", {
                  input: { command: "printf failure; exit 7" },
                  output: {
                    exit_code: 7,
                    stdout: "actual command output",
                    stderr: "failed",
                  },
                }),
                trace("unknown", "bash", "sandbox", {
                  status: "failed",
                  rawStatus: "result_unknown",
                  summary: "result_unknown",
                }),
                trace("cancelled", "read", "sandbox", {
                  status: "failed",
                  rawStatus: "cancelled",
                }),
              ],
            });
            db.messages.push({
              ...base,
              id: "middle",
              content: "Second real model segment",
            });
            db.messages.push({
              ...base,
              id: "second-activities",
              content: "",
              toolCalls: [
                trace("next-one", "tool.search", "argus"),
                trace("next-two", "tool.describe", "argus"),
              ],
            });
            db.messages.push({
              ...base,
              id: "after",
              content: "Final model answer",
            });
            localStorage.setItem(key, JSON.stringify(db));
          },
          { conversationId },
        );
        await page.reload();
        const groups = page.getByTestId("tool-activity-group");
        await expect(groups).toHaveCount(2);
        const group = groups.first(),
          toggle = group.locator(":scope > button");
        await expect(toggle).toHaveAttribute("aria-expanded", "false");
        await expect(toggle).toContainText(english ? "Failed 1" : "失败 1");
        await expect(toggle).toContainText(
          english ? "Result unknown 1" : "结果未知 1",
        );
        await toggle.focus();
        await page.keyboard.press("Enter");
        await expect(toggle).toHaveAttribute("aria-expanded", "true");
        const rows = group.locator(".argus-tool-activity");
        await expect(rows).toHaveCount(7);
        const alignment = await rows.evaluateAll((nodes) =>
          nodes.map((node) => {
            const meta = node.querySelector(".argus-tool-activity__meta")!;
            const duration = node.querySelector(
              ".argus-tool-activity__duration",
            )!;
            const arrow = meta.querySelector(".argus-tool-activity__chevron")!;
            return {
              time: duration.getBoundingClientRect().right,
              arrow: arrow.getBoundingClientRect().right,
            };
          }),
        );
        expect(
          Math.max(...alignment.map((row) => row.time)) -
            Math.min(...alignment.map((row) => row.time)),
        ).toBeLessThanOrEqual(1);
        expect(
          Math.max(...alignment.map((row) => row.arrow)) -
            Math.min(...alignment.map((row) => row.arrow)),
        ).toBeLessThanOrEqual(1);
        await expect(
          rows.filter({ hasText: english ? "Read file" : "读取文件" }).first(),
        ).toContainText("input.csv");

        // Exactly the trusted workspace edit has native file semantics.
        await expect(
          group.locator('.argus-tool-activity[data-kind="file_change"]'),
        ).toHaveCount(1);
        const edit = group.locator(
          '.argus-tool-activity[data-kind="file_change"]',
        );
        await expect(edit).toContainText("+1");
        await edit.getByRole("button").click();
        await expect(
          edit.getByRole("region", {
            name: english ? "Change diff" : "变更差异",
          }),
        ).toContainText("actual new line");
        const external = rows.filter({ hasText: /^write/ });
        await external.getByRole("button").click();
        await expect(external).toContainText(
          english ? "External parameter tool" : "外部参数工具",
        );
        await expect(external).not.toContainText("+999");
        await external
          .getByText(english ? "Input parameters" : "输入参数", { exact: true })
          .click();
        await expect(external.locator("details[open] pre")).toContainText(
          "/workspace/forged.md",
        );
        const inputColors = await external
          .locator("details[open] pre")
          .evaluate((el) => ({
            background: getComputedStyle(el).backgroundColor,
            expected: getComputedStyle(document.documentElement)
              .getPropertyValue("--bg-muted")
              .trim(),
            color: getComputedStyle(el).color,
          }));
        expect(inputColors.background).not.toBe("rgb(12, 11, 10)");
        const command = rows.filter({ hasText: "printf failure; exit 7" });
        await expect(command).toHaveAttribute("data-state", "failed");
        await command.getByRole("button").click();
        await expect(command).toContainText(
          english ? "Exit code: 7" : "退出码：7",
        );
        const size = await toggle.evaluate((el) => ({
          font: parseFloat(getComputedStyle(el).fontSize),
          height: el.getBoundingClientRect().height,
        }));
        expect(size.font).toBeLessThanOrEqual(13);
        expect(size.height).toBeLessThanOrEqual(36);
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth,
          ),
        ).toBe(true);
        await page.screenshot({
          path: info.outputPath("activities.png"),
          animations: "disabled",
        });
        await toggle.click();
        await expect(toggle).toHaveAttribute("aria-expanded", "false");
        await expect(
          page.getByText("Second real model segment", { exact: true }),
        ).toBeVisible();
      });
    }
