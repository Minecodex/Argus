import { expect, test } from "@playwright/test";
import { readFileSync, writeFileSync } from "node:fs";
import { setChoice } from "./helpers/choice";
import {
  dashboardBinding,
  openSealedDashboard,
} from "./helpers/sealed-dashboard";

for (const english of [false, true])
  for (const dark of [false, true]) {
    for (const kind of ["plan", "user_input"]) {
      test(`host delivery ${kind} ${english ? "en" : "zh"} ${dark ? "dark" : "light"}`, async ({
        page,
      }, info) => {
        await page.setViewportSize({ width: 1440, height: 900 });
        await openSealedDashboard(page, english, dark, dashboardBinding.Data);
        const conversation = new URL(page.url()).searchParams.get("c")!;
        const id = await page.evaluate(
          ({ conversation, kind }) => {
            const key = "argus-mock:db-v14",
              db = JSON.parse(localStorage.getItem(key)!);
            const id = crypto.randomUUID(),
              now = new Date().toISOString();
            db.messages = db.messages.filter(
              (row: { conversationId: string }) =>
                row.conversationId !== conversation,
            );
            db.conversationRuns = (db.conversationRuns ?? []).filter(
              (row: { conversation_id: string }) =>
                row.conversation_id !== conversation,
            );
            db.interactions = (db.interactions ?? []).filter(
              (row: { conversation_id: string }) =>
                row.conversation_id !== conversation,
            );
            db.interactions.push({
              id,
              family_id: id,
              conversation_id: conversation,
              run_id: crypto.randomUUID(),
              kind,
              status: "pending",
              revision: 1,
              version: 1,
              content_hash: "a".repeat(64),
              created_at: now,
              updated_at: now,
              content:
                kind === "plan"
                  ? {
                      title: "Observed repair plan",
                      document_markdown:
                        "# Observed repair plan\n\nInspect evidence before any change.\n\n```bash\nprintf evidence-only\n```",
                    }
                  : {
                      questions: [
                        {
                          id: "scope",
                          header: "Scope",
                          question: "Choose the authorized resource",
                          options: [
                            {
                              label: "Owned resource",
                              description: "Use the current grant",
                            },
                            {
                              label: "Narrow the range",
                              description: "Ask for more detail",
                            },
                          ],
                        },
                      ],
                    },
            });
            localStorage.setItem(key, JSON.stringify(db));
            return id;
          },
          { conversation, kind },
        );
        await page.reload();
        const state = () =>
          page.evaluate(
            (id) =>
              JSON.parse(
                localStorage.getItem("argus-mock:db-v14")!,
              ).interactions.find((row: { id: string }) => row.id === id),
            id,
          );
        if (kind === "plan") {
          const document = page.getByRole("article", {
            name: "Observed repair plan",
          });
          await expect(document).toContainText(
            "Inspect evidence before any change.",
          );
          await expect(document).toContainText("printf evidence-only");
          await expect(
            page.getByRole("button", {
              name: english ? "Implement this plan" : "执行此计划",
              exact: true,
            }),
          ).toBeVisible();
          const download = page.waitForEvent("download");
          await document
            .getByRole("button", {
              name: english ? "Download plan file" : "下载计划文件",
            })
            .click();
          const file = await download;
          expect(file.suggestedFilename()).toBe("plan-v1.md");
          await file.saveAs(info.outputPath("plan-v1.md"));
          expect(readFileSync(info.outputPath("plan-v1.md"), "utf8")).toBe(
            (await state()).content.document_markdown,
          );
          expect((await state()).status).toBe("pending");
          expect((await state()).implementation_run_id).toBeUndefined();
          await page.reload();
          await expect(document).toContainText("printf evidence-only");
          expect((await state()).status).toBe("pending");
        } else {
          const option = page.getByRole("radio", { name: /Owned resource/ });
          await expect(option).toBeVisible();
          await setChoice(option);
          expect((await state()).status).toBe("pending");
          await page
            .getByRole("button", {
              name: english ? "Submit answers" : "提交回答",
              exact: true,
            })
            .click();
          await expect
            .poll(async () => (await state()).status)
            .toBe("answered");
          expect((await state()).response.answers.scope.selected_option).toBe(
            "Owned resource",
          );
          await page.reload();
          await expect(
            page.getByTestId("chat-interaction-event"),
          ).toContainText("Owned resource");
        }
        await expect(
          page.locator(".argus-tool-presentation iframe"),
        ).toHaveCount(0);
        await page.screenshot({ path: info.outputPath(`${kind}.png`) });
        writeFileSync(
          info.outputPath("receipt.json"),
          JSON.stringify(
            {
              passed: true,
              layer: "L4",
              controlled_host_delivery: true,
              kind,
              locale: english ? "en-US" : "zh-CN",
              theme: dark ? "dark" : "light",
              no_template_iframe: true,
              implicit_submission: false,
              actual_remote_execution_tested: false,
            },
            null,
            2,
          ),
        );
      });
    }
  }
