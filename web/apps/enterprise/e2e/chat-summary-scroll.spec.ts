import { expect, test } from "@playwright/test";
import { createHash } from "node:crypto";

// Historical-style source deliberately keeps its old height limits. Runtime must
// lay it out as a document without changing this source or hiding any content.
const source = `<section id="content"></section><style>
.argus-template-code{max-height:320px;overflow:auto}
.argus-dashboard-grid{max-height:1600px;overflow:auto}
</style><script>ArgusTemplate.onData(() => {
 const root=document.getElementById('content'); root.replaceChildren();
 const d=document.createElement('details'); d.open=true;
 const s=document.createElement('summary'); s.textContent='Long saved result'; d.append(s);
 const pre=document.createElement('pre');pre.className='argus-template-code';
 pre.textContent=Array.from({length:180},(_,i)=>'Line '+i+' · saved query configuration').join('\\n');d.append(pre);root.append(d);
 const scroll=document.createElement('div'); scroll.className='argus-observation-table';
 const table=document.createElement('table');const row=document.createElement('tr');
 for(let i=0;i<30;i++){const td=document.createElement('td');td.textContent='Column '+i+' stored observation';row.append(td);}
 table.append(row);scroll.append(table);root.append(scroll);
 const end=document.createElement('p');end.textContent='End of saved result';root.append(end);
});</script>`;
const hash = createHash("sha256").update(source).digest("hex");

for (const english of [false, true])
  for (const dark of [false, true])
    for (const viewport of [
      { width: 1280, height: 800 },
      { width: 1600, height: 1000 },
      { width: 1920, height: 1080 },
    ])
      test(`pinned summary and full document ${english ? "en" : "zh"} ${dark ? "dark" : "light"} ${viewport.width}`, async ({
        page,
      }, info) => {
        test.setTimeout(90000);
        await page.setViewportSize(viewport);
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
        await composer.getByRole("textbox").fill("summary layout verification");
        await composer
          .getByRole("button", { name: english ? "Send" : "发送", exact: true })
          .click();
        await expect(page).toHaveURL(/c=/);
        await expect(composer.getByRole("textbox")).toHaveValue("");
        const id = new URL(page.url()).searchParams.get("c")!;
        await page.evaluate(
          ({ id, source, hash }) => {
            const key = "argus-mock:db-v14",
              db = JSON.parse(localStorage.getItem(key)!);
            const conversation = db.conversations.find(
              (c: { id: string }) => c.id === id,
            );
            const now = new Date().toISOString();
            db.messages = db.messages.filter(
              (m: { conversationId: string }) => m.conversationId !== id,
            );
            db.messages.push({
              id: "long-result",
              conversationId: id,
              role: "assistant",
              content: "",
              createdAt: now,
              toolCalls: [
                {
                  callId: "long-result",
                  toolName: "host.list",
                  source: "argus",
                  status: "success",
                  startedAt: now,
                },
              ],
              presentations: ["long-result"],
            });
            db.planv5.presentations["long-result"] = {
              conversationId: id,
              value: {
                tool_call_id: "long-result",
                status: "ready",
                runtime: "argus-template/v1",
                template_source: source,
                template_hash: hash,
                detail_data: {},
              },
            };
            db.planv5.files[id] = Array.from({ length: 24 }, (_, i) => ({
              id: `file-${i}`,
              name: `${String(i).padStart(2, "0")} · report & <readonly> ${"long-name-".repeat(5)}.txt`,
              path: `/workspace/file-${i}.txt`,
              media_type: "text/plain",
              byte_size: 13,
              created_at: now,
            }));
            for (const file of db.planv5.files[id])
              db.planv5.contents[file.id] =
                "data:text/plain;base64,c2F2ZWQgcmVzdWx0";
            db.planv5.workspaces[id] = {
              id: "workspace",
              conversation_id: id,
              status: "ready",
              version: 1,
              capacity_bytes: 2147483648,
              created_at: now,
            };
            db.planv5.connections = [
              {
                enterpriseId: conversation.enterpriseId,
                value: {
                  id: "mcp-summary",
                  name: "Read-only MCP",
                  endpoint: "https://example.test/mcp",
                  member_ids: [conversation.createdBy],
                  status: "enabled",
                  tool_count: 3,
                  revision: 1,
                  version: 1,
                  created_at: now,
                  updated_at: now,
                },
              },
            ];
            localStorage.setItem(key, JSON.stringify(db));
          },
          { id, source, hash },
        );
        await page.reload();
        const trigger = page.getByRole("button", {
          name: english ? "Pinned summary" : "置顶摘要",
          exact: true,
        });
        await expect(
          page.locator(".argus-chat__main > .argus-conversation-workspace"),
        ).toHaveCount(0);
        await expect(
          page.locator("iframe.argus-tool-presentation__frame"),
        ).toHaveCount(0);
        await trigger.click();
        const popup = page.getByRole("dialog", {
          name: english ? "Pinned summary" : "置顶摘要",
          exact: true,
        });
        await expect(popup).toBeVisible();
        await expect(
          popup
            .locator(".argus-conversation-workspace > details > summary")
            .last(),
        ).toContainText("24");
        const connection = popup.getByRole("checkbox", {
          name: /Read-only MCP/,
        });
        await popup.getByText(/Read-only MCP/).click();
        await expect(connection).toBeChecked();
        const body = popup.locator(".argus-content-popover__body");
        await expect
          .poll(() => body.evaluate((el) => el.scrollHeight > el.clientHeight))
          .toBe(true);
        const dimensions = await popup.boundingBox();
        expect(dimensions!.y).toBeGreaterThan(0);
        expect(dimensions!.y + dimensions!.height).toBeLessThan(
          viewport.height,
        );
        expect(
          await body.evaluate((el) => el.scrollWidth - el.clientWidth),
        ).toBeLessThanOrEqual(1);
        await page.screenshot({ path: info.outputPath("summary.png") });
        await popup
          .getByRole("button", { name: english ? /^Preview / : /^预览 / })
          .first()
          .click();
        const preview = page.getByRole("dialog").filter({
          has: page.getByText(english ? "Read-only preview" : "只读在线预览", {
            exact: false,
          }),
        });
        await expect(preview).toBeVisible();
        await expect(preview).toContainText("saved result");
        await page.keyboard.press("Escape");
        await expect(preview).not.toBeVisible();
        await expect(popup).toBeVisible();
        await expect(
          popup
            .getByRole("button", { name: english ? /^Preview / : /^预览 / })
            .first(),
        ).toBeFocused();
        await page.keyboard.press("Escape");
        await expect(popup).not.toBeVisible();
        await expect(trigger).toBeFocused();
        await page.locator(".argus-tool-result__header button").click();
        const frame = page.locator("iframe.argus-tool-presentation__frame");
        await expect(frame).toBeVisible({ timeout: 25000 });
        const content = frame.contentFrame();
        await expect
          .poll(async () => (await frame.boundingBox())!.height)
          .toBeGreaterThan(3000);
        await expect
          .poll(() =>
            content
              .locator("html")
              .evaluate((el) => el.scrollHeight - el.clientHeight),
          )
          .toBeLessThanOrEqual(1);
        expect(
          await content
            .locator("pre")
            .evaluate((el) => el.scrollHeight - el.clientHeight),
        ).toBeLessThanOrEqual(1);
        const horizontal = content.locator(".argus-observation-table");
        expect(
          await horizontal.evaluate((el) => el.scrollWidth > el.clientWidth),
        ).toBe(true);
        await horizontal.evaluate((el) => (el.scrollLeft = 200));
        expect(
          await horizontal.evaluate((el) => el.scrollLeft),
        ).toBeGreaterThan(0);
        const scrollSize = await page
          .locator(".argus-chat__stream")
          .evaluate((el) => getComputedStyle(el, "::-webkit-scrollbar").width);
        expect(scrollSize).toBe("9px");
        expect(
          await horizontal.evaluate(
            (el) => getComputedStyle(el, "::-webkit-scrollbar").height,
          ),
        ).toBe(scrollSize);
        await content.getByText("End of saved result").scrollIntoViewIfNeeded();
        await page.screenshot({
          path: info.outputPath("full-document-end.png"),
        });
        await content.getByText("Long saved result", { exact: true }).click();
        await expect
          .poll(async () => (await frame.boundingBox())!.height)
          .toBeLessThan(500);
        await expect
          .poll(() =>
            content
              .locator("html")
              .evaluate((el) => el.scrollHeight - el.clientHeight),
          )
          .toBeLessThanOrEqual(1);
        const stream = await page.locator(".argus-chat__stream").boundingBox(),
          input = await composer.boundingBox();
        expect(stream!.y + stream!.height).toBeLessThanOrEqual(input!.y + 1);
        await page.reload();
        await trigger.click();
        await expect(connection).toBeChecked();
        await page.keyboard.press("Escape");
      });
