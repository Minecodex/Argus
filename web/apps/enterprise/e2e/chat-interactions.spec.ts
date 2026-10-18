import { test, expect, type Page } from "@playwright/test";
import { readFile } from "node:fs/promises";
async function expectReadingColumn(page: Page, selector: string) {
  const reference = page.getByTestId("chat-assistant-heading").first();
  await expect(reference).toBeVisible();
  const heading = (await reference.boundingBox())!;
  for (const item of await page.locator(selector).all()) {
    const box = (await item.boundingBox())!;
    expect(Math.abs(box.x - heading.x)).toBeLessThanOrEqual(1);
    expect(Math.abs(box.width - heading.width)).toBeLessThanOrEqual(1);
    expect(
      await item.evaluate(
        (element) => element.scrollWidth <= element.clientWidth + 1,
      ),
    ).toBe(true);
  }
}
async function seedInteraction(
  page: Page,
  kind: "plan" | "question",
  revision = 1,
) {
  await page.evaluate(
    ({ kind, revision }) => {
      const key = "argus-mock:db-v14",
        db = JSON.parse(localStorage.getItem(key)!);
      const conversation = new URL(location.href).searchParams.get("c")!;
      const id = kind === "plan" ? `draft-${revision}` : "questions";
      const base = {
        id,
        family_id: kind === "plan" ? "plan-family" : id,
        conversation_id: conversation,
        run_id: "interaction-run",
        kind: kind === "question" ? "user_input" : "plan",
        revision,
        version: 1,
        status: "pending",
        content_hash: "a".repeat(64),
        created_at: new Date(Date.now() - 100).toISOString(),
        updated_at: new Date().toISOString(),
      };
      const content =
        kind === "plan"
          ? {
              title: `Repair revision ${revision}`,
              document_markdown: `# Repair revision ${revision}

Diagnose and repair only the owned fixture. /tmp/${"long-path-segment".repeat(24)}

## Inspect

**待执行 / Execute · Owned host · operator**

\`\`\`bash argus:inspect
printf 'fixture inspection'
printf '${"long-command".repeat(30)}'\n\`\`\`

## Verify

\`\`\`powershell argus:verify
Get-Content $env:TEMP/fixture.txt\n\`\`\``,
            }
          : {
              questions: [
                {
                  id: "scope",
                  header: "Scope",
                  question: "Which target should be used?",
                  options: [
                    {
                      label: "Owned fixture",
                      description: "Temporary test resources only",
                    },
                    { label: "Custom", description: "Describe the target" },
                  ],
                },
                {
                  id: "notes",
                  header: "Notes",
                  question: "What constraints should be followed?",
                  options: [],
                },
              ],
            };
      db.interactions ??= [];
      if (kind === "plan")
        for (const item of db.interactions)
          if (item.kind === "plan" && item.status === "pending")
            item.status = "superseded";
      db.interactions.push({ ...base, content });
      localStorage.setItem(key, JSON.stringify(db));
    },
    { kind, revision },
  );
  await page.reload();
}
for (const english of [false, true])
  for (const theme of ["light", "dark"])
    for (const viewport of [
      { width: 1280, height: 800 },
      { width: 1440, height: 900 },
      { width: 1920, height: 1080 },
    ]) {
      test.describe(`chat interaction ${english ? "en" : "zh"} ${theme} ${viewport.width}`, () => {
        test.use({ viewport });
        test("menus, persisted permissions, plan revisions, confirmation and real user answers", async ({
          page,
        }, info) => {
          test.setTimeout(90000);
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
          await expect(page).not.toHaveURL(/\/login/);
          await page.goto("/");
          const composer = page.locator(".argus-chat-composer");
          await composer
            .getByRole("button", {
              name: english ? "Add" : "添加",
              exact: true,
            })
            .click();
          await expect(page.getByRole("menu")).toContainText(
            english ? "Skills" : "技能",
          );
          await page
            .getByRole("menuitem", {
              name: english ? /^Create dashboard/ : /^创建仪表盘/,
            })
            .click();
          await expect(composer.getByRole("textbox")).not.toHaveValue("");
          await composer
            .getByRole("button", {
              name: english ? "Add" : "添加",
              exact: true,
            })
            .click();
          await page
            .getByRole("menuitem", {
              name: english ? /^Plan mode/ : /^计划模式/,
            })
            .click();
          await expect(page).toHaveURL(/c=/);
          await expect(
            composer.getByRole("button", {
              name: english ? "Exit plan mode" : "退出计划模式",
            }),
          ).toBeVisible();
          await composer
            .getByRole("button", {
              name: english ? "Request approval" : "请求审批",
              exact: true,
            })
            .click();
          await page
            .getByRole("menuitem", {
              name: english ? /^Full access/ : /^完全访问/,
            })
            .click();
          await expect(
            composer.getByRole("button", {
              name: english ? "Full access" : "完全访问",
              exact: true,
            }),
          ).toBeVisible();
          await page.reload();
          await expect(
            composer.getByRole("button", {
              name: english ? "Full access" : "完全访问",
              exact: true,
            }),
          ).toBeVisible();
          await composer
            .getByRole("button", {
              name: english ? "Add" : "添加",
              exact: true,
            })
            .click();
          await page
            .getByRole("menuitem", {
              name: english ? /^Reference a resource/ : /^引用资源/,
            })
            .click();
          await expect(page.getByRole("listbox")).toBeVisible();
          await composer.getByRole("textbox").press("Escape");
          await composer
            .getByRole("button", {
              name: english ? "Add" : "添加",
              exact: true,
            })
            .click();
          const chooser = page.waitForEvent("filechooser");
          await page
            .getByRole("menuitem", {
              name: english ? /Attach|Upload/ : /上传|添加附件/,
            })
            .click();
          await (
            await chooser
          ).setFiles({
            name: "interaction-fixture.log",
            mimeType: "text/plain",
            buffer: Buffer.from("owned fixture\n"),
          });
          await expect(
            composer
              .locator(".argus-chat-chip")
              .filter({ hasText: "interaction-fixture.log" }),
          ).toBeVisible();
          await seedInteraction(page, "plan");
          await expect(page.locator(".argus-guidance-plan")).toHaveCount(1);
          await expect(page.locator(".argus-guidance-plan")).toContainText(
            "printf 'fixture inspection'",
          );
          await expectReadingColumn(page, ".argus-guidance-plan");
          const planDocument = page.locator(".argus-guidance-plan");
          await expect(planDocument.locator("pre code")).toHaveCount(2);
          await expect(
            planDocument.locator(".argus-code__head"),
          ).toHaveCount(0);
          await expect(
            planDocument.getByRole("button", { name: /停止|Stop/ }),
          ).toHaveCount(0);
          const downloadEvent = page.waitForEvent("download");
          await planDocument
            .getByRole("button", {
              name: english ? "Download plan file" : "下载计划文件",
            })
            .click();
          const download = await downloadEvent;
          expect(download.suggestedFilename()).toBe("plan-v1.md");
          expect(await readFile((await download.path())!, "utf8")).toContain(
            "```bash argus:inspect",
          );
          await expect(page.locator(".argus-composer-panel")).toBeVisible();
          await composer
            .getByRole("textbox", {
              name: english ? "Revision feedback" : "修改意见",
            })
            .fill("Add a verification step before changing anything");
          await composer
            .getByRole("button", {
              name: english ? "Submit feedback" : "提交修改意见",
            })
            .click();
          await expect(page.locator(".argus-guidance-plan")).toHaveCount(0);
          await seedInteraction(page, "plan", 2);
          await expect(page.locator(".argus-guidance-plan")).toHaveCount(1);
          await expect(page.locator(".argus-guidance-plan")).toContainText(
            "Repair revision 2",
          );
          await expectReadingColumn(page, ".argus-guidance-plan");
          await composer
            .getByRole("button", {
              name: english ? "Collapse" : "收起",
              exact: true,
            })
            .click();
          await expect(page.locator(".argus-composer-panel")).toBeHidden();
          await page.reload();
          await expect(page.locator(".argus-composer-panel")).toBeVisible();
          await composer
            .getByRole("button", {
              name: english ? "Implement this plan" : "执行此计划",
              exact: true,
            })
            .click();
          await expect(page.locator(".argus-composer-panel")).toHaveCount(0);
          await expect(
            composer.getByRole("button", {
              name: english ? "Exit plan mode" : "退出计划模式",
            }),
          ).toHaveCount(0);
          await expect(
            page.getByTestId("chat-message-user").last(),
          ).toContainText("是的，执行此计划");
          // The implementation Run is a new server-side task, not the planning
          // stream held in this page. Reload must recover its stop control.
          await page.reload();
          const stop = composer.getByRole("button", {
            name: english ? "Stop task" : "停止任务",
            exact: true,
          });
          await expect(stop).toBeVisible();
          await stop.click();
          await expect(stop).toHaveCount(0);
          await expect(page.locator(".argus-guidance-plan")).toContainText(
            english ? "Confirmed" : "已确认",
          );
          await page.reload();
          await expect(stop).toHaveCount(0);
          await expect(page.locator(".argus-guidance-plan")).toContainText(
            english ? "Confirmed" : "已确认",
          );
          await seedInteraction(page, "plan", 3);
          await expect(page.locator(".argus-guidance-plan")).toHaveCount(2);
          // Keep the confirmed plan, replace the unconfirmed draft with a question fixture.
          await page.evaluate(() => {
            const key = "argus-mock:db-v14",
              db = JSON.parse(localStorage.getItem(key)!);
            for (const item of db.interactions)
              if (item.status === "pending") item.status = "superseded";
            localStorage.setItem(key, JSON.stringify(db));
          });
          await seedInteraction(page, "question");
          await expect(page.locator(".argus-guidance-plan")).toHaveCount(1);
          await expect(page.locator(".argus-composer-panel")).toContainText(
            "Which target should be used?",
          );
          await page.getByRole("radio", { name: /^Owned fixture/ }).check();
          await composer
            .getByRole("button", {
              name: english ? "Next question" : "下一题",
              exact: true,
            })
            .last()
            .click();
          await composer
            .getByRole("textbox", {
              name: english ? "Additional answer" : "补充回答",
            })
            .fill(
              `Only the temporary directory /tmp/${"long-answer-segment".repeat(24)}`,
            );
          await composer
            .getByRole("button", {
              name: english ? "Submit answers" : "提交回答",
              exact: true,
            })
            .click();
          await expect(page.locator(".argus-question-summary")).toContainText(
            english ? "Asked 2 questions" : "已询问 2 个问题",
          );
          await page.locator(".argus-question-summary summary").click();
          await expect(page.locator(".argus-question-summary")).toContainText(
            "Only the temporary directory",
          );
          await expectReadingColumn(
            page,
            ".argus-guidance-plan, .argus-question-summary",
          );
          await page.reload();
          await expect(page.locator(".argus-question-summary")).toContainText(
            english ? "Asked 2 questions" : "已询问 2 个问题",
          );
          await expect(page.locator(".argus-composer-panel")).toHaveCount(0);
          await page.screenshot({
            path: info.outputPath("interactions.png"),
            fullPage: true,
            animations: "disabled",
          });
        });
      });
    }
