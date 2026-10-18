import { expect, test } from "@playwright/test";

const longName = `${"long-report-".repeat(12)}&<readonly>.txt`;
const fixtures = [
  {
    id: "root-note",
    path: "/workspace/readme.txt",
    content: "Workspace root note",
  },
  {
    id: "report-ten",
    path: "/workspace/reports/2026/report10.txt",
    content: "Report ten",
  },
  {
    id: "report-two",
    path: "/workspace/reports/2026/report2.txt",
    content: "Report two",
  },
  {
    id: "report-log",
    path: "/workspace/reports/2026/stdout.txt",
    content: "Report command output",
  },
  {
    id: "execution-log",
    path: "/workspace/executions/step-2/stdout.txt",
    content: "Execution command output",
  },
  {
    id: "long-name",
    path: `/workspace/reports/2026/${longName}`,
    content: "Long filename content",
  },
];

for (const english of [false, true])
  for (const dark of [false, true])
    for (const width of [1280, 1920])
      test(`workspace directory browser ${english ? "en" : "zh"} ${dark ? "dark" : "light"} ${width}`, async ({
        page,
      }, info) => {
        await page.setViewportSize({ width, height: 900 });
        await page.addInitScript(
          ({ english, dark }) => {
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
        await composer.getByRole("textbox").fill("Workspace directory browser");
        await composer
          .getByRole("button", { name: english ? "Send" : "发送", exact: true })
          .click();
        await expect(page).toHaveURL(/c=/);
        await expect(composer.getByRole("textbox")).toHaveValue("");
        const conversationId = new URL(page.url()).searchParams.get("c")!;
        await page.evaluate(
          ({ conversationId, fixtures }) => {
            const key = "argus-mock:db-v14";
            const db = JSON.parse(localStorage.getItem(key)!);
            const created_at = new Date().toISOString();
            db.planv5.files[conversationId] = fixtures.map(
              ({ id, path, content }) => {
                db.planv5.contents[id] =
                  `data:text/plain;base64,${btoa(content)}`;
                return {
                  id,
                  path,
                  name: path.split("/").at(-1),
                  byte_size: content.length,
                  media_type: "text/plain",
                  created_at,
                };
              },
            );
            db.planv5.workspaces[conversationId] = {
              id: "directory-workspace",
              conversation_id: conversationId,
              status: "ready",
              version: 1,
              capacity_bytes: 2147483648,
              created_at,
            };
            localStorage.setItem(key, JSON.stringify(db));
          },
          { conversationId, fixtures },
        );
        await page.reload();
        const trigger = page.getByRole("button", {
          name: english ? "Pinned summary" : "置顶摘要",
          exact: true,
        });
        await trigger.click();
        const popup = page.getByRole("dialog", {
          name: english ? "Pinned summary" : "置顶摘要",
          exact: true,
        });
        const tree = popup.getByRole("list", {
          name: english ? "Conversation files" : "会话文件",
          exact: true,
        });
        await expect(tree).toBeVisible();
        await expect(
          popup.getByRole("button", { name: /删除|Delete/i }),
        ).toHaveCount(0);
        await expect(popup).toContainText(
          english
            ? "Files stay with the conversation and are removed when you permanently delete it."
            : "文件随会话保留，永久删除会话时一并清理。",
        );
        await expect(
          popup.getByText(
            `${english ? "Conversation files" : "会话文件"} · 6`,
            { exact: true },
          ),
        ).toBeVisible();
        await expect(popup.locator(".argus-file-card")).toHaveCount(0);
        const row = (name: string) =>
          tree.locator(".argus-file-row").filter({
            has: page.getByRole("button", {
              name: `${english ? "Preview" : "预览"} ${name}`,
              exact: true,
            }),
          });
        await expect(row("readme.txt")).toBeVisible();
        await expect(row("report2.txt")).not.toBeVisible();
        await expect(
          tree.locator(":scope > li > details > ul > li > details > summary"),
        ).toHaveText(["executions", "reports"]);

        const reports = tree.locator('summary[title="/workspace/reports"]');
        await reports.click();
        const year = tree.locator('summary[title="/workspace/reports/2026"]');
        await year.focus();
        await page.keyboard.press("Enter");
        await expect(row("report2.txt")).toBeVisible();
        const names = await tree
          .locator('summary[title="/workspace/reports/2026"]')
          .locator("..")
          .locator(".argus-file-row__preview")
          .allTextContents();
        expect(names.indexOf("report2.txt")).toBeLessThan(
          names.indexOf("report10.txt"),
        );
        await expect(row(longName)).toBeVisible();
        const dimensions = await row(longName).boundingBox();
        expect(dimensions!.height).toBeLessThanOrEqual(30);
        const name = row(longName).locator(".argus-file-tree__name");
        expect(
          await name.evaluate(
            (element) => element.scrollWidth > element.clientWidth,
          ),
        ).toBe(true);
        await page.screenshot({ path: info.outputPath("workspace-tree.png") });
        await popup.screenshot({
          path: info.outputPath("workspace-tree-panel.png"),
        });
        expect(
          await popup
            .locator(".argus-content-popover__body")
            .evaluate((element) => element.scrollWidth - element.clientWidth),
        ).toBeLessThanOrEqual(1);

        const preview = row("report2.txt").getByRole("button");
        await preview.click();
        const dialog = page.getByRole("dialog", {
          name: "report2.txt",
          exact: true,
        });
        await expect(dialog).toContainText("Report two");
        await page.keyboard.press("Escape");
        await expect(preview).toBeFocused();
        const link = row("report2.txt").getByRole("link", {
          name: `${english ? "Download" : "下载"} report2.txt`,
          exact: true,
        });
        await expect(link).toHaveAttribute("download", "report2.txt");
        const [download] = await Promise.all([
          page.waitForEvent("download"),
          link.click(),
        ]);
        expect(download.suggestedFilename()).toBe("report2.txt");
        const downloaded = await download.createReadStream();
        const chunks = [];
        for await (const chunk of downloaded!) chunks.push(chunk);
        expect(Buffer.concat(chunks).toString()).toBe("Report two");

        await reports.click();
        await expect(row("report2.txt")).not.toBeVisible();
        await reports.click();
        await expect(row("report2.txt")).toBeVisible();
        await tree.locator('summary[title="/workspace/executions"]').click();
        await tree
          .locator('summary[title="/workspace/executions/step-2"]')
          .click();
        await expect(row("stdout.txt")).toHaveCount(2);
        await row("stdout.txt").last().getByRole("button").click();
        await expect(
          page.getByRole("dialog", { name: "stdout.txt", exact: true }),
        ).toContainText("Report command output");
        await page.keyboard.press("Escape");
        await tree.locator('summary[title="/workspace"]').click();
        await expect(row("readme.txt")).not.toBeVisible();
        await tree.locator('summary[title="/workspace"]').click();
        await expect(row("readme.txt")).toBeVisible();
        await page.reload();
        await trigger.click();
        await expect(row("readme.txt")).toBeVisible();
        await expect(row("report2.txt")).not.toBeVisible();
      });
