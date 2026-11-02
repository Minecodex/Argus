import { expect, test } from "@playwright/test";
import { writeFileSync } from "node:fs";
import { openToolResults } from "./helpers/tool-results";
const enabled = process.env.ARGUS_PLAN11_DASHBOARD_DISCOVERY_REAL === "1";
const cases = enabled
  ? (JSON.parse(process.env.ARGUS_PLAN11_DISCOVERY_CASES!) as {
      name: string;
      conversation_id: string;
    }[])
  : [];
test.skip(
  !enabled,
  "requires owned actual model/dashboard discovery conversations",
);
test.use({ trace: "off", viewport: { width: 1440, height: 1000 } });
for (const locale of ["zh-CN", "en-US"])
  for (const theme of ["light", "dark"]) {
    test(`Actual dashboard discovery ${locale} ${theme}`, async ({
      page,
      context,
    }, info) => {
      test.setTimeout(150_000);
      await context.addCookies(
        JSON.parse(process.env.ARGUS_PLAN11_DISCOVERY_COOKIES!),
      );
      await page.addInitScript(
        ({ locale, theme }) => {
          localStorage.setItem("argus.locale", locale);
          localStorage.setItem("argus.theme", theme);
        },
        { locale, theme },
      );
      expect(cases).toHaveLength(2);
      for (const row of cases) {
        await page.goto(`/?c=${row.conversation_id}`);
        await expect(page).not.toHaveURL(/\/login/);
        await openToolResults(page);
        const frames = page.locator(".argus-tool-presentation__frame");
        await expect.poll(() => frames.count()).toBeGreaterThan(0);
        await expect
          .poll(async () => {
            for (let index = 0; index < await frames.count(); index++)
              if (
                await frames.nth(index).contentFrame()
                  .locator("body")
                  .innerText()
                  .catch(() => "")
                  .then((text) =>
                    text.includes(
                      locale === "zh-CN" ? "部分适用" : "Partially applicable",
                    ),
                  )
              )
                return true;
            return false;
          })
          .toBe(true);
        let metadataNote = false;
        for (let index = 0; index < await frames.count(); index++) {
          const text = await frames.nth(index).contentFrame()
            .locator("body")
            .innerText()
            .catch(() => "");
          if (
            text.includes(
              locale === "zh-CN" ? "未执行面板采样" : "panels were not sampled",
            )
          )
            metadataNote = true;
        }
        expect(metadataNote).toBe(true);
        await expect(page.getByTestId("pending-action-card")).toHaveCount(0);
        await page.screenshot({ path: info.outputPath(`${row.name}.png`) });
      }
      writeFileSync(
        info.outputPath("receipt.json"),
        JSON.stringify({
          passed: true,
          locale,
          theme,
          actual_conversations: cases.map((row) => row.conversation_id),
          real_api_model_template: true,
          partial_suitability_visible: true,
          metadata_not_sampled_visible: true,
        }),
      );
    });
  }
