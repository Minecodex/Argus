import { expect, test } from "@playwright/test";
import { createHash } from "node:crypto";
import AxeBuilder from "@axe-core/playwright";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 conversation secure form ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      await page.setViewportSize({ width: 1440, height: 1000 });
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
      const composer = page.locator(".argus-chat-composer");
      await composer.getByRole("textbox").fill("Monitoring form verification");
      await composer
        .getByRole("button", { name: english ? "Send" : "发送", exact: true })
        .click();
      await expect(page).toHaveURL(/c=/);
      await expect(composer.getByRole("textbox")).toHaveValue("");
      const id = new URL(page.url()).searchParams.get("c")!;
      const source = "<p>Monitoring form request</p>";
      await page.evaluate(
        ({ id, source, hash }) => {
          const key = "argus-mock:db-v14",
            db = JSON.parse(localStorage.getItem(key)!);
          const now = new Date().toISOString(),
            call = "host-form-call";
          db.messages = db.messages.filter(
            (item: { conversationId: string }) => item.conversationId !== id,
          );
          db.messages.push({
            id: call,
            conversationId: id,
            role: "assistant",
            content: "",
            createdAt: now,
            toolCalls: [
              {
                callId: call,
                toolName: "monitoring.collection.form",
                source: "argus",
                status: "success",
                startedAt: now,
              },
            ],
            presentations: [call],
          });
          db.planv5.presentations[call] = {
            conversationId: id,
            value: {
              tool_call_id: call,
              status: "ready",
              runtime: "argus-template/v2",
              template_source: source,
              template_hash: hash,
              detail_data: {
                tool: "monitoring.collection.form",
                schema_version: "argus.monitoring_host_form/v1",
                integration: "mysql",
                status: "requires_user_input",
                prefill: {
                  name: "Chat orders",
                  endpoint: "mysql.chat.test:3306",
                  database: "",
                },
              },
            },
          };
          localStorage.setItem(key, JSON.stringify(db));
        },
        { id, source, hash: createHash("sha256").update(source).digest("hex") },
      );
      await page.reload();
      await page.getByRole("button", { name: /^(查看|View) · / }).click();
      const open = page.getByRole("button", {
        name: english ? "Open secure monitoring form" : "打开监控安全表单",
      });
      await expect(open).toBeVisible();
      await open.click();
      const form = page.getByRole("dialog").last();
      await expect(
        form.getByRole("textbox", {
          name: new RegExp(english ? "Monitor name" : "监控名称"),
        }),
      ).toHaveValue("Chat orders");
      await expect(
        form.getByRole("textbox", {
          name: new RegExp(english ? "Service endpoint" : "服务地址"),
        }),
      ).toHaveValue("mysql.chat.test:3306");
      await form
        .getByRole("textbox", {
          name: new RegExp(english ? "Username" : "用户名"),
        })
        .fill("chat-monitor");
      const privateValue = "private-conversation-form-canary";
      await form.locator('input[type="password"]').fill(privateValue);
      await form
        .getByRole("button", {
          name: english ? "Choose a Collector" : "选择执行收集器",
          exact: true,
        })
        .click();
      const collectors = page.getByRole("dialog", {
        name: english ? "Choose a Collector" : "选择执行收集器",
        exact: true,
      });
      await collectors
        .getByRole("button", { name: /^(选择|Select) / })
        .first()
        .click();
      expect(
        (await new AxeBuilder({ page }).analyze()).violations.filter(
          (item) => item.impact === "critical" || item.impact === "serious",
        ),
      ).toEqual([]);
      await page.screenshot({
        path: info.outputPath("secure-form.png"),
        fullPage: true,
      });
      await form
        .getByRole("button", {
          name: english ? "Preview configuration" : "预览采集配置",
          exact: true,
        })
        .click();
      await expect(page.locator('input[type="password"]')).toHaveCount(0);
      await expect(page.getByText(privateValue, { exact: false })).toHaveCount(
        0,
      );
      await page
        .getByRole("button", {
          name: english ? "Confirm changes" : "确认变更",
          exact: true,
        })
        .click();
      await expect(
        page.getByRole("button", {
          name: english ? "Confirm changes" : "确认变更",
          exact: true,
        }),
      ).toHaveCount(0);
      const persisted = await page.evaluate(() =>
        localStorage.getItem("argus-mock:db-v14")!,
      );
      expect(persisted).not.toContain(privateValue);
      expect(persisted).toContain("Chat orders");
    });
  }
