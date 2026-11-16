import { expect, test } from "@playwright/test";
import { plan7Login } from "./helpers/plan7-login";

test.skip(
  process.env.ARGUS_CONTEXT_RUNTIME_REAL !== "1",
  "Requires the deployed context runtime and real B12 records; read-only browser review",
);
test.use({
  ignoreHTTPSErrors: false,
  trace: "off",
  video: "off",
  reducedMotion: "reduce",
});

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`deployed context runtime history ${english ? "en" : "zh"} ${theme}`, async ({
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
      await plan7Login(
        page,
        "/login",
        "kakj",
        process.env.ARGUS_PLAN9_PASSWORD ?? "",
      );
      await page.goto("/?c=01a1e1f0-4809-78f5-bb8b-9b7b96e35a44");
      const version = page
        .getByTestId("tool-trace")
        .filter({
          hasText: english ? "Condition version conflict" : "条件版本不一致",
        })
        .first();
      await expect(version).toBeVisible();
      await expect(version).toContainText("dashboard/context.resolve");
      const evidence = page
        .getByTestId("tool-trace")
        .filter({
          hasText: english
            ? "Condition evidence is invalid"
            : "条件证据不符合要求",
        })
        .first();
      await expect(evidence).toBeVisible();
      await expect(evidence).toContainText("dashboard/context.resolve");
      await evidence.getByRole("button").click();
      await expect(evidence).toContainText("tool.invoke");
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      await page.screenshot({
        path: info.outputPath("real-specific-tool-errors.png"),
        animations: "disabled",
      });
      const conversation = process.env.ARGUS_CONTEXT_RUNTIME_CONVERSATION;
      if (conversation) {
        await page.goto("/?c=" + encodeURIComponent(conversation));
        const name =
          process.env.ARGUS_CONTEXT_RUNTIME_REPORT ??
          "b12-context-verification.md";
        const report = page
          .getByRole("button", {
            name: `${english ? "Preview" : "预览"} ${name}`,
            exact: true,
          })
          .last();
        await expect(report).toBeVisible({ timeout: 20000 });
        await report.click();
        const dialog = page.getByRole("dialog", { name, exact: true });
        await expect(dialog.locator(".argus-markdown")).toBeVisible();
        await expect(dialog).toContainText("command_delta");
        await expect(dialog).toContainText("1562696");
        await expect(dialog).toContainText('"target_id": "a"');
        await expect(dialog).toContainText('"resource_id"');
        await page.screenshot({
          path: info.outputPath("real-compressed-analysis-report.png"),
          animations: "disabled",
        });
      }
    });
  }
