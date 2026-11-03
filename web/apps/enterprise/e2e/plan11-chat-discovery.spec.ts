import { test, expect, type Page } from "@playwright/test";
import { randomUUID } from "node:crypto";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { openToolResults } from "./helpers/tool-results";

const fixture = JSON.parse(
  readFileSync(
    resolve(process.cwd(), "../template-runtime/src/fixtures.generated.json"),
    "utf8",
  ),
);
async function login(page: Page, english: boolean, theme: string) {
  await page.addInitScript(
    ({ english, theme }) => {
      if (window !== window.top) return;
      localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
      localStorage.setItem("argus.theme", theme);
    },
    { english, theme },
  );
  await page.goto("/login");
  await page.getByLabel(english ? "Username" : "用户名").fill("root");
  await page.getByLabel(english ? "Password" : "密码").fill("123456");
  await page
    .getByRole("button", { name: english ? "Sign in" : "登录", exact: true })
    .click();
  await expect(page).not.toHaveURL(/\/login/);
}
for (const english of [false, true])
  for (const theme of ["light", "dark"])
    for (const width of [1280, 1440, 1920]) {
      test(`Plan11 typed references, scrolling and discovery views ${english ? "en" : "zh"} ${theme} ${width}`, async ({
        page,
      }, info) => {
        await page.setViewportSize({ width, height: 1000 });
        await login(page, english, theme);
        const targets = Array.from({ length: 35 }, () => randomUUID()).sort(),
          app = targets[34]!;
        await page.evaluate(
          ({ app, targets }) => {
            const key = "argus-mock:db-v14",
              db = JSON.parse(localStorage.getItem(key)!);
            const hint = JSON.parse(
              localStorage.getItem("argus.auth_hint.enterprise")!,
            );
            const member = db.enterpriseUsers.find(
              (row: { userId: string }) => row.userId === hint.user_id,
            );
            if (!member)
              throw new Error("mock enterprise identity unavailable");
            db.monitoring ??= {};
            const domain = (db.monitoring[member.enterpriseId] ??= {
              applications: [],
              services: [],
              bindings: [],
              targets: [],
            });
            domain.applications.push({
              id: app,
              application_key: app,
              name: "plan11-orders",
              environment: "production",
              description: "",
              owner_name: "",
              status: "active",
              version: 1,
              created_at: new Date().toISOString(),
              updated_at: new Date().toISOString(),
            });
            domain.targets ??= [];
            db.dataAuthorizationGrants ??= [];
            for (const [index, id] of targets.entries()) {
              domain.targets.push({
                id,
                name: `plan11-target-${String(index).padStart(2, "0")}`,
                kind: "database",
                engine: "mysql",
                identity: {},
                status: "active",
                version: 1,
              });
              db.dataAuthorizationGrants.push({
                subject_type: "user",
                subject_id: hint.user_id,
                resource_type: "monitoring_target",
                resource_id: id,
                active: true,
              });
            }
            db.dataAuthorizationGrants.push({
              subject_type: "user",
              subject_id: hint.user_id,
              resource_type: "application",
              resource_id: app,
              active: true,
            });
            localStorage.setItem(key, JSON.stringify(db));
          },
          { app, targets },
        );
        await page.reload();
        const composer = page.locator(".argus-chat-composer"),
          area = composer.getByRole("textbox");
        await area.fill("@plan11-target");
        const picker = page.getByRole("listbox");
        await expect(
          picker.getByRole("option", { name: /plan11-target-00/ }),
        ).toBeVisible();
        await expect(picker.getByRole("option")).toHaveCount(20);
        await picker.locator(".argus-chat-picker__list").evaluate((node) => {
          node.scrollTop = node.scrollHeight;
        });
        await expect(picker.getByRole("option")).toHaveCount(35);
        await picker.getByRole("option", { name: /plan11-target-34/ }).click();
        await area.fill(`@${app}`);
        await expect(
          picker.getByRole("option", { name: /plan11-orders/ }),
        ).toBeVisible();
        await area.press("Enter");
        await expect(picker).toHaveCount(0);
        await expect(composer.locator(".argus-chat-chip")).toHaveCount(2);
        await composer
          .locator(".argus-chat-chip")
          .filter({ hasText: "plan11-orders" })
          .getByRole("button")
          .click();
        await expect(composer.locator(".argus-chat-chip")).toHaveCount(1);
        await expect(composer.locator(".argus-chat-chip")).toContainText(
          "plan11-target-34",
        );
        await area.fill(`@${app}`);
        await expect(
          picker.getByRole("option", { name: /plan11-orders/ }),
        ).toBeVisible();
        await area.press("Enter");
        await expect(picker).toHaveCount(0);
        await area.fill(
          english ? "Inspect these referenced resources" : "检查这些引用的资源",
        );
        await composer
          .getByRole("button", { name: english ? "Send" : "发送", exact: true })
          .click();
        await expect(page).toHaveURL(/c=/);
        await expect(area).toHaveValue("", { timeout: 20000 });
        const conversation = new URL(page.url()).searchParams.get("c")!;
        await expect
          .poll(() =>
            page.evaluate(
              ({ conversation }) => {
                const db = JSON.parse(
                  localStorage.getItem("argus-mock:db-v14")!,
                );
                return db.messages.find(
                  (row: { conversationId: string; role: string }) =>
                    row.conversationId === conversation && row.role === "user",
                )?.dashboardContext?.resource_refs;
              },
              { conversation },
            ),
          )
          .toEqual([
            { type: "monitoring_target", id: targets[34] },
            { type: "application", id: app },
          ]);
        const names = [
          "observe.resources.list",
          "observe.capabilities.get",
          "observe.catalog.query",
        ];
        const presentations = names.map((name) => {
          const binding = fixture.bindings.find(
            (row: { Tool: string }) => row.Tool === name,
          );
          return {
            id: randomUUID(),
            tool: name,
            source: fixture.assets[binding.Hash],
            hash: binding.Hash,
            reads: binding.Reads,
            data: binding.Data,
          };
        });
        await page.evaluate(
          ({ conversation, presentations }) => {
            const key = "argus-mock:db-v14",
              db = JSON.parse(localStorage.getItem(key)!);
            db.messages = db.messages.filter(
              (row: { conversationId: string; role: string }) =>
                row.conversationId !== conversation || row.role === "user",
            );
            for (const p of presentations) {
              db.planv5.presentations[p.id] = {
                conversationId: conversation,
                value: {
                  tool_call_id: p.id,
                  runtime: "argus-template/v2",
                  read_operations: p.reads,
                  status: "ready",
                  template_source: p.source,
                  template_hash: p.hash,
                  detail_data: p.data,
                  resource_refs: [],
                  result_refs: [],
                },
              };
              db.messages.push({
                id: p.id,
                conversationId: conversation,
                role: "assistant",
                content: "",
                createdAt: new Date().toISOString(),
                toolCalls: [
                  {
                    callId: p.id,
                    toolName: p.tool,
                    source: "argus",
                    status: "success",
                    startedAt: new Date().toISOString(),
                  },
                ],
                presentations: [p.id],
              });
            }
            localStorage.setItem(key, JSON.stringify(db));
          },
          { conversation, presentations },
        );
        await page.reload();
        await openToolResults(page);
        const frames = page.locator(".argus-tool-presentation__frame");
        await expect(frames).toHaveCount(3);
        await expect(
          frames.nth(0).contentFrame().locator("body"),
        ).toContainText("orders-mysql");
        await expect(
          frames.nth(1).contentFrame().locator("body"),
        ).toContainText(english ? "Partial" : "结果不完整");
        await expect(
          frames.nth(2).contentFrame().locator("body"),
        ).toContainText("mysql_threads");
        await expect
          .poll(() =>
            page.evaluate(
              () => document.documentElement.scrollWidth <= innerWidth + 1,
            ),
          )
          .toBe(true);
        await page.screenshot({
          path: info.outputPath("plan11-discovery.png"),
          animations: "disabled",
        });
      });
    }
