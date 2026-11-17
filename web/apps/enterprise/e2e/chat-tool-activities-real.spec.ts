import { expect, test } from "@playwright/test";
import { plan7Login } from "./helpers/plan7-login";
import { openSummarySection } from "./helpers/conversation-summary";

test.skip(
  process.env.ARGUS_CHAT_ACTIVITIES_REAL !== "1",
  "Existing B12 normal-login acceptance",
);
test.use({
  ignoreHTTPSErrors: false,
  trace: "off",
  video: "off",
  reducedMotion: "reduce",
});
for (const width of [1280, 1440, 1920])
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`real B12 activity groups ${width} ${english ? "en" : "zh"} ${theme}`, async ({
        page,
      }, info) => {
        await page.setViewportSize({ width, height: 1000 });
        await page.addInitScript(
          ({ english, theme }) => {
            if (window.self !== window.top) return;
            localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
            localStorage.setItem("argus.theme", theme);
          },
          { english, theme },
        );
        let runMutations = 0;
        page.on("request", (request) => {
          if (
            request.method() !== "GET" &&
            /\/(messages|preflight|access-mode|mode)$/.test(
              new URL(request.url()).pathname,
            )
          )
            runMutations++;
        });
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        await plan7Login(
          page,
          "/login",
          "kakj",
          process.env.ARGUS_PLAN9_PASSWORD ?? "",
        );
        await page.goto(
          "/?c=" +
            encodeURIComponent(
              process.env.ARGUS_CHAT_ACTIVITIES_CONVERSATION ??
                "01a1e422-2c55-74de-9aa0-05d2e37987b6",
            ),
        );
        await expect(
          page.getByTestId("chat-assistant-reply").first(),
        ).toBeVisible();
        const groups = page.getByTestId("tool-activity-group");
        await expect(groups.first()).toBeVisible();
        const toggle = groups.first().locator(":scope > button");
        await expect(toggle).toHaveAttribute("aria-expanded", "false");
        await toggle.click();
        await expect(toggle).toHaveAttribute("aria-expanded", "true");
        const rows = groups.first().locator(".argus-tool-activity");
        expect(await rows.count()).toBeGreaterThanOrEqual(2);
        const positions = await rows.evaluateAll((nodes) =>
          nodes.map((node) => ({
            arrow: node
              .querySelector(".argus-tool-activity__chevron")!
              .getBoundingClientRect().right,
            time: node
              .querySelector(".argus-tool-activity__duration")!
              .getBoundingClientRect().right,
          })),
        );
        expect(
          Math.max(...positions.map((p) => p.arrow)) -
            Math.min(...positions.map((p) => p.arrow)),
        ).toBeLessThanOrEqual(1);
        expect(
          Math.max(...positions.map((p) => p.time)) -
            Math.min(...positions.map((p) => p.time)),
        ).toBeLessThanOrEqual(1);
        const row = rows.first(),
          button = row.locator(":scope > button");
        await button.click();
        await expect(button).toHaveAttribute("aria-expanded", "true");
        await expect(
          row.locator(".argus-tool-activity__provenance"),
        ).toContainText(english ? /tool|record/ : /工具|记录/);
        const geometry = await toggle.evaluate((el) => ({
          width: el.getBoundingClientRect().width,
          height: el.getBoundingClientRect().height,
          font: parseFloat(getComputedStyle(el).fontSize),
          overflow: el.scrollWidth > el.clientWidth,
        }));
        expect(geometry.font).toBeLessThanOrEqual(13);
        expect(geometry.height).toBeLessThanOrEqual(36);
        expect(geometry.overflow).toBe(false);
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth,
          ),
        ).toBe(true);
        await page.screenshot({
          path: info.outputPath("real-b12-activities.png"),
          animations: "disabled",
        });
        const business = groups.first().getByTestId("tool-result").first();
        if (await business.count()) {
          const header = business.locator(".argus-tool-activity > button");
          if ((await header.getAttribute("aria-expanded")) === "false")
            await header.click();
          await business
            .getByTestId("tool-business-view")
            .locator(":scope > summary")
            .click();
          const frame = business.locator(
            "iframe.argus-tool-presentation__frame",
          );
          await expect(frame).toBeVisible({ timeout: 25000 });
          await expect
            .poll(async () => (await frame.boundingBox())?.width ?? 0)
            .toBeGreaterThan(geometry.width + 24);
          await page.screenshot({
            path: info.outputPath("real-wide-view.png"),
            animations: "disabled",
          });
          await business
            .getByTestId("tool-business-view")
            .locator(":scope > summary")
            .click();
        }
        await toggle.click();
        await expect(toggle).toHaveAttribute("aria-expanded", "false");
        await expect(
          page.locator(".argus-chat-message__body").first(),
        ).toBeVisible();
        const composer = page.locator(".argus-chat-composer"),
          model = composer
            .locator(".argus-chat-composer__model")
            .getByRole("button");
        await expect(model).toBeVisible();
        const modelSpacing = await model.evaluate((el) => {
          const label = el
              .querySelector(".argus-select__value")!
              .getBoundingClientRect(),
            arrow = el
              .querySelector(".argus-select__indicator")!
              .getBoundingClientRect();
          return {
            gap: arrow.left - label.right,
            width: el.getBoundingClientRect().width,
          };
        });
        expect(modelSpacing.gap).toBeGreaterThanOrEqual(0);
        expect(modelSpacing.gap).toBeLessThanOrEqual(12);
        await expect(model).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
        await expect(
          page.locator(".argus-chat__header .argus-select"),
        ).toHaveCount(0);
        await model.click();
        const menu = page.getByRole("listbox");
        await expect(menu).toBeVisible();
        await expect
          .poll(async () => {
            const box = (await menu.boundingBox())!,
              anchor = (await model.boundingBox())!;
            return box.y + box.height - anchor.y;
          })
          .toBeLessThanOrEqual(1);
        await page.screenshot({
          path: info.outputPath("real-model-picker.png"),
          animations: "disabled",
        });
        await page.keyboard.press("Escape");
        const summary = page.getByRole("button", {
          name: english ? "Pinned summary" : "置顶摘要",
          exact: true,
        });
        await summary.click();
        const popup = page.getByRole("dialog", {
          name: english ? "Pinned summary" : "置顶摘要",
          exact: true,
        });
        await expect(
          popup.locator(".argus-conversation-workspace > details[open]"),
        ).toHaveCount(0);
        await openSummarySection(page, "resources");
        const resources = page.getByTestId("conversation-resources-section");
        await expect(resources.getByRole("table")).toBeVisible();
        expect(await resources.locator("tbody tr").count()).toBeLessThanOrEqual(
          5,
        );
        const first = resources.getByRole("link").first();
        await expect(first).toBeVisible();
        const resourceName = (await first.textContent())!;
        await resources.getByRole("searchbox").fill(resourceName);
        await expect(first).toContainText(resourceName);
        await page.screenshot({
          path: info.outputPath("real-summary-table.png"),
          animations: "disabled",
        });
        await page.keyboard.press("Escape");
        await summary.click();
        await expect(
          popup.locator(".argus-conversation-workspace > details[open]"),
        ).toHaveCount(0);
        await page.keyboard.press("Escape");
        expect(runMutations).toBe(0);
        expect(errors).toEqual([]);
        await info.attach("read-only-proof", {
          body: JSON.stringify({
            groups: await groups.count(),
            runMutations,
            errors,
            geometry,
            width,
            english,
            theme,
          }),
          contentType: "application/json",
        });
      });
    }
