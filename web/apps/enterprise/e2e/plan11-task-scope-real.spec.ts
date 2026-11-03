import { expect, test } from "@playwright/test";
import { writeFileSync } from "node:fs";
test.skip(
  process.env.ARGUS_PLAN11_SCOPE_REAL !== "1",
  "requires owned real-model persisted task-scope conversation",
);
test.use({ trace: "off", viewport: { width: 1440, height: 1000 } });
for (const locale of ["zh-CN", "en-US"])
  for (const theme of ["light", "dark"]) {
    test(`Actual persistent task scope ${locale} ${theme}`, async ({
      page,
      context,
    }, info) => {
      await context.addCookies(
        JSON.parse(process.env.ARGUS_PLAN11_SCOPE_COOKIES!),
      );
      await page.addInitScript(
        ({ locale, theme }) => {
          localStorage.setItem("argus.locale", locale);
          localStorage.setItem("argus.theme", theme);
        },
        { locale, theme },
      );
      const id = process.env.ARGUS_PLAN11_SCOPE_CONVERSATION!;
      const expected = process.env.ARGUS_PLAN11_EXPECTED_SCOPE!;
      await page.goto(`/?c=${id}`);
      await expect(page).not.toHaveURL(/\/login/);
      const badge = page.getByText(
        locale === "zh-CN"
          ? "仅分析已发布仪表盘"
          : "Published dashboard analysis only",
        { exact: true },
      );
      await expect(badge.first()).toBeVisible();
      await page.reload();
      await expect(badge.first()).toBeVisible();
      const selection = await page.evaluate(async (id) => {
        const response = await fetch(
          `/api/v1/conversations/${id}/dashboard-context`,
          { credentials: "include" },
        );
        return {
          status: response.status,
          value: response.ok ? await response.json() : null,
        };
      }, id);
      expect(selection.status).toBe(200);
      expect(JSON.stringify(selection.value)).toContain(expected);
      if (expected === "general_diagnosis")
        await expect(
          page
            .getByTestId("chat-message-user")
            .filter({ hasText: "/综合诊断" }),
        ).toBeVisible();
      await page.screenshot({ path: info.outputPath("scope.png") });
      writeFileSync(
        info.outputPath("receipt.json"),
        JSON.stringify({
          passed: true,
          locale,
          theme,
          expected,
          visible_historical_restriction: true,
          survives_reload: true,
          current_api_scope_matches: true,
          actual_model_conversation: id,
        }),
      );
    });
  }
