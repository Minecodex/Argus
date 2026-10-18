import { test, expect } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { plan7Login } from "./helpers/plan7-login";

const enabled = process.env.ARGUS_CHAT_LAYOUT_REAL === "1";
for (const locale of ["zh-CN", "en-US"])
  for (const theme of ["light", "dark"])
    for (const viewport of [
      { width: 1280, height: 800 },
      { width: 1440, height: 900 },
      { width: 1920, height: 1080 },
    ]) {
      test.describe(`existing chat reading column ${locale} ${theme} ${viewport.width}`, () => {
        test.skip(
          !enabled,
          "Requires an existing authorized conversation; no model calls or remote actions are created",
        );
        test.use({ viewport });
        test("aligns plans, question summaries, execution cards and files", async ({
          page,
        }, info) => {
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
          await page.goto(`/?c=${process.env.ARGUS_CHAT_LAYOUT_CONVERSATION!}`);
          await expect(page.locator(".argus-guidance-plan")).toHaveCount(1);
          await expect(
            page.locator(".argus-command-execution").first(),
          ).toBeVisible();
          await expect(
            page.locator(".argus-question-summary").first(),
          ).toBeVisible();
          await page.locator(".argus-question-summary summary").first().click();
          // Failed preparation calls have no action record and remain visible.
          await expect(
            page
              .getByTestId("tool-trace")
              .filter({ hasText: /远程命令|Remote command/ })
              .filter({
                has: page.locator(".argus-chat-trace__status.is-success"),
              }),
          ).toHaveCount(0);
          const reference = (await page
            .getByTestId("chat-assistant-heading")
            .first()
            .boundingBox())!;
          const selectors = [
            ".argus-guidance-plan",
            ".argus-question-summary",
            ".argus-command-execution",
            ".argus-chat-message:not(.argus-chat-message--user) > .argus-chat-message__body",
            ".argus-chat-activity > .argus-chat-files",
            ".argus-chat-activity > .argus-chat-files-group",
          ];
          const measurements = [];
          for (const selector of selectors) {
            for (const item of await page.locator(selector).all()) {
              const box = (await item.boundingBox())!;
              expect(
                Math.abs(box.x - reference.x),
                selector,
              ).toBeLessThanOrEqual(1);
              expect(
                Math.abs(box.width - reference.width),
                selector,
              ).toBeLessThanOrEqual(1);
              expect(
                await item.evaluate(
                  (el) => el.scrollWidth <= el.clientWidth + 1,
                ),
                selector,
              ).toBe(true);
              measurements.push({ selector, x: box.x, width: box.width });
            }
          }
          await writeFile(
            info.outputPath("reading-column.json"),
            JSON.stringify(
              { locale, theme, viewport, reference, measurements },
              null,
              2,
            ),
          );
          const plan = page.locator(".argus-guidance-plan");
          await plan.evaluate((element) =>
            element.scrollIntoView({ block: "start" }),
          );
          await page.screenshot({
            path: info.outputPath("plan-alignment.png"),
            animations: "disabled",
          });
          await page
            .locator(".argus-question-summary")
            .first()
            .evaluate((element) => element.scrollIntoView({ block: "center" }));
          await page.screenshot({
            path: info.outputPath("question-alignment.png"),
            animations: "disabled",
          });
        });
      });
    }
