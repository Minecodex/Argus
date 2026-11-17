import { expect, test } from "@playwright/test";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`tool failure identity and field feedback ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      await page.setViewportSize({ width: 1440, height: 900 });
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
      await page.locator('form button[type="submit"]').click();
      const composer = page.locator(".argus-chat-composer");
      await composer.getByRole("textbox").fill("Tool failure review");
      await composer
        .getByRole("button", { name: english ? "Send" : "发送", exact: true })
        .click();
      await expect(page).toHaveURL(/c=/);
      await expect(composer.getByRole("textbox")).toHaveValue("");
      const id = new URL(page.url()).searchParams.get("c")!;
      await page.evaluate(
        ({ id }) => {
          const key = "argus-mock:db-v14",
            db = JSON.parse(localStorage.getItem(key)!);
          db.messages = db.messages.filter(
            (m: { conversationId: string }) => m.conversationId !== id,
          );
          for (const [name, code, details] of [
            [
              "context.resolve",
              "DASHBOARD_CONDITION_EVIDENCE_REQUIRED",
              {
                issues: [
                  {
                    path: "/evidence/0/quote",
                    constraint: "exact_excerpt_from_current_user_message",
                  },
                ],
              },
            ],
            ["get", "DASHBOARD_VERSION_CONFLICT", { actual_version: 0 }],
          ] as const) {
            db.messages.push({
              id: "failed-" + name,
              conversationId: id,
              role: "assistant",
              content: "",
              createdAt: new Date().toISOString(),
              toolCalls: [
                {
                  callId: "call-" + name,
                  toolName: "tool.invoke",
                  source: "argus",
                  status: "failed",
                  startedAt: new Date().toISOString(),
                  input: { category: "dashboard", name, arguments: {} },
                  output: { error_code: code, details },
                },
              ],
            });
          }
          localStorage.setItem(key, JSON.stringify(db));
        },
        { id },
      );
      await page.reload();
      const group = page.getByTestId("tool-activity-group");
      await group.locator(":scope > button").click();
      const trace = page
        .getByTestId("tool-trace")
        .filter({ hasText: "dashboard/context.resolve" });
      await expect(trace).toContainText(
        english ? "Condition evidence is invalid" : "条件证据不符合要求",
      );
      await expect(
        page.getByTestId("tool-trace").filter({ hasText: "dashboard/get" }),
      ).toContainText(english ? "server: 0" : "服务端：0");
      await trace.getByRole("button").click();
      await expect(trace.getByRole("table")).toContainText("/evidence/0/quote");
      await expect(trace.getByRole("table")).toContainText(
        english ? "exact contiguous excerpt" : "必须逐字引用",
      );
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      await page.screenshot({
        path: info.outputPath("tool-failure-feedback.png"),
        animations: "disabled",
      });
    });
  }
