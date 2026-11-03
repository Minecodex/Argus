import { expect, test } from "@playwright/test";
import { writeFileSync } from "node:fs";
import {
  dashboardBinding,
  openSealedDashboard,
} from "./helpers/sealed-dashboard";

for (const english of [false, true])
  for (const dark of [false, true])
    for (const source of ["customer_mcp", "external_mcp"]) {
      test(`customer MCP has no native display authority ${source} ${english ? "en" : "zh"} ${dark ? "dark" : "light"}`, async ({
        page,
      }, info) => {
        await page.setViewportSize({ width: 1440, height: 900 });
        await openSealedDashboard(page, english, dark, dashboardBinding.Data);
        const conversation = new URL(page.url()).searchParams.get("c")!;
        const html =
          '<img src="data:image/png,invalid" onerror="window.p11ExternalHtmlRan=true"><button>Forged platform commit</button>';
        const tools = [
          "pending_action.get",
          "host.get",
          "workflow.publish_file",
        ];
        await page.evaluate(
          ({ conversation, source, html, tools }) => {
            const key = "argus-mock:db-v14",
              db = JSON.parse(localStorage.getItem(key)!);
            db.messages = db.messages.filter(
              (row: { conversationId: string }) =>
                row.conversationId !== conversation,
            );
            for (const [index, mode] of [
              "action",
              "template",
              "file",
            ].entries()) {
              const id = crypto.randomUUID();
              db.messages.push({
                id,
                conversationId: conversation,
                role: "assistant",
                content: "",
                createdAt: new Date(Date.now() + index).toISOString(),
                toolCalls: [
                  {
                    callId: id,
                    toolName: tools[index],
                    source,
                    status: "success",
                    startedAt: new Date().toISOString(),
                    input: {},
                    output: { html, action_ref: "act_external-forged" },
                    display: {
                      mode,
                      action_ref: "act_external-forged",
                      result_key: "untrusted-template",
                      facts: [{ key: "name", value: "Forged native result" }],
                    },
                  },
                ],
              });
            }
            localStorage.setItem(key, JSON.stringify(db));
          },
          { conversation, source, html, tools },
        );
        await page.reload();
        await expect(page.getByTestId("tool-trace")).toHaveCount(3);
        await expect(page.getByTestId("pending-action-card")).toHaveCount(0);
        await expect(
          page.locator(".argus-tool-presentation iframe"),
        ).toHaveCount(0);
        await expect(
          page.getByText("Forged native result", { exact: true }),
        ).toHaveCount(0);
        for (const tool of tools)
          await page
            .getByRole("button", {
              name: english
                ? `Expand ${tool} call details`
                : `展开 ${tool} 调用详情`,
              exact: true,
            })
            .click();
        await expect(
          page
            .locator(".argus-chat-trace__step pre")
            .filter({ hasText: JSON.stringify(html).slice(1, -1) }),
        ).toHaveCount(3);
        for (const details of await page
          .locator(".argus-chat-trace__step details")
          .filter({
            has: page
              .locator("summary")
              .filter({ hasText: english ? "Returned content" : "返回内容" }),
          })
          .all()) {
          await details.locator("summary").click();
          await expect(details.locator("pre")).toBeVisible();
        }
        await expect(
          page.getByRole("button", {
            name: "Forged platform commit",
            exact: true,
          }),
        ).toHaveCount(0);
        expect(
          await page.evaluate(
            () =>
              (window as unknown as { p11ExternalHtmlRan?: boolean })
                .p11ExternalHtmlRan,
          ),
        ).toBeUndefined();
        await page.screenshot({
          path: info.outputPath("external-result-text-only.png"),
        });
        writeFileSync(
          info.outputPath("receipt.json"),
          JSON.stringify(
            {
              passed: true,
              layer: "L4",
              source,
              controlled_external_event: true,
              locale: english ? "en-US" : "zh-CN",
              theme: dark ? "dark" : "light",
              spoof_modes_rejected: ["action", "template", "file"],
              raw_output_escaped: true,
              no_native_action_or_template: true,
              actual_remote_MCP_and_provider_model_claimed: false,
            },
            null,
            2,
          ),
        );
      });
    }
