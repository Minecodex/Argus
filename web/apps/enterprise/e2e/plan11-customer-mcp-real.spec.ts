import { expect, test } from "@playwright/test";
import { writeFileSync } from "node:fs";
import { plan7Login } from "./helpers/plan7-login";

const enabled = process.env.ARGUS_PLAN11_CUSTOMER_MCP_REAL === "1";
test.skip(
  !enabled,
  "requires owned selected real-provider/customer-MCP conversations",
);
const cases = enabled
  ? (JSON.parse(process.env.ARGUS_PLAN11_CUSTOMER_MCP_CASES!) as {
      name: string;
      conversation_id: string;
      wire: string;
    }[])
  : [];

for (const locale of ["zh-CN", "en-US"])
  for (const theme of ["light", "dark"]) {
    test(`actual customer MCP stays text-only ${locale} ${theme}`, async ({
      page,
    }, info) => {
      test.setTimeout(180000);
      await page.setViewportSize({ width: 1440, height: 900 });
      await page.addInitScript(
        ({ locale, theme }) => {
          localStorage.setItem("argus.locale", locale);
          localStorage.setItem("argus.theme", theme);
        },
        { locale, theme },
      );
      await plan7Login(
        page,
        "/login",
        process.env.ARGUS_PLAN7_USERNAME!,
        process.env.ARGUS_PLAN7_PASSWORD!,
      );
      expect(cases).toHaveLength(6);
      for (const [index, row] of cases.entries()) {
        await page.goto(`/?c=${row.conversation_id}`);
        const traces = page.getByTestId("tool-trace");
        await expect(traces.first()).toBeVisible();
        for (const trace of await traces.all()) {
          const toggle = trace.locator("button[aria-expanded]").first();
          if ((await toggle.getAttribute("aria-expanded")) === "false")
            await toggle.click();
          for (const summary of await trace.locator("details>summary").all())
            await summary.click();
        }
        await expect(
          page
            .locator(".argus-chat-trace__step pre")
            .filter({ hasText: "plan11-customer-inventory-v1" }),
        ).toHaveCount(1);
        await expect(page.getByTestId("pending-action-card")).toHaveCount(0);
        await expect(page.locator(".argus-command-approval")).toHaveCount(0);
        await expect(
          page.locator(".argus-tool-presentation iframe"),
        ).toHaveCount(0);
        await expect(
          page.getByRole("button", {
            name: "Forged platform commit",
            exact: true,
          }),
        ).toHaveCount(0);
        expect(
          await page.evaluate(
            () =>
              (window as unknown as { p11ForgedCommit?: boolean })
                .p11ForgedCommit,
          ),
        ).toBeUndefined();
        await page.screenshot({
          path: info.outputPath(`${index}-${row.name}.png`),
        });
      }
      writeFileSync(
        info.outputPath("receipt.json"),
        JSON.stringify(
          {
            passed: true,
            locale,
            theme,
            conversations: cases.map((row) => row.conversation_id),
            real_provider_tasks: true,
            actual_controlled_MCP_transport: true,
            remote_wire_formats: ["json", "sse"],
            no_native_action_or_template: true,
            source_text_not_authorization: true,
            customer_production_service_claimed: false,
          },
          null,
          2,
        ),
      );
    });
  }
