import { expect, test } from "@playwright/test";
import { plan7Login } from "./helpers/plan7-login";

const conversation = process.env.ARGUS_PLAN9_PREVIEW_CONVERSATION ?? "";
test.skip(
  process.env.ARGUS_PLAN9_PREVIEW_REAL !== "1",
  "Requires an existing real conversation and published report; makes no model calls",
);
test.use({
  ignoreHTTPSErrors: false,
  trace: "off",
  video: "off",
  reducedMotion: "reduce",
});

for (const [width, english, theme] of [
  [1280, false, "light"],
  [1440, true, "dark"],
  [1920, false, "dark"],
] as const) {
  test(`real attachment, delivery and Workspace previews ${width} ${theme}`, async ({
    page,
  }, info) => {
    test.setTimeout(120000);
    await page.setViewportSize({ width, height: 1000 });
    await page.addInitScript(
      ({ english, theme }) => {
        localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
        localStorage.setItem("argus.theme", theme);
      },
      { english, theme },
    );
    let modelCalls = 0;
    page.on("request", (request) => {
      if (
        request.method() === "POST" &&
        /\/(messages|preflight)$/.test(new URL(request.url()).pathname)
      )
        modelCalls++;
    });
    await plan7Login(
      page,
      "/login",
      process.env.ARGUS_PLAN9_USERNAME ?? "kakj",
      process.env.ARGUS_PLAN9_PASSWORD ?? "",
    );
    await page.goto(`/?c=${encodeURIComponent(conversation)}`);
    const preview = english ? "Preview" : "预览";
    const raw = english ? "Raw" : "原文";
    await page
      .getByRole("button", {
        name: `${preview} b12-native-redis.csv`,
        exact: true,
      })
      .first()
      .click();
    let dialog = page.getByRole("dialog", {
      name: "b12-native-redis.csv",
      exact: true,
    });
    await expect(dialog.getByRole("table")).toBeVisible();
    await expect(dialog.getByRole("columnheader")).toHaveCount(5);
    await expect(dialog.locator("tbody tr")).toHaveCount(8);
    await expect(dialog.getByRole("table")).toContainText("1562696");
    await expect(dialog.getByRole("table")).toContainText("1489");
    await dialog.getByRole("button", { name: raw, exact: true }).click();
    await expect(dialog.locator("pre")).toContainText(
      "observed_at,used_memory_bytes,total_commands_processed",
    );
    await dialog.getByRole("button", { name: preview, exact: true }).click();
    await page.screenshot({
      path: info.outputPath("b12-real-csv.png"),
      animations: "disabled",
    });
    await page.keyboard.press("Escape");
    await page
      .getByRole("button", {
        name: `${preview} B12 Redis INFO 附件分析报告`,
        exact: true,
      })
      .first()
      .click();
    dialog = page.getByRole("dialog", {
      name: "B12 Redis INFO 附件分析报告",
      exact: true,
    });
    await expect(
      dialog.getByRole("heading", {
        name: "B12 真实场景验收 — Redis 原生 INFO 附件分析",
        exact: true,
      }),
    ).toBeVisible();
    await expect(
      dialog.locator("pre").filter({ hasText: '"command_delta": 37' }),
    ).toContainText('"peak_memory_bytes": 1562696');
    await expect(dialog.locator("ul").first()).toBeVisible();
    await page.screenshot({
      path: info.outputPath("b12-real-markdown.png"),
      animations: "disabled",
    });
    await dialog.getByRole("button", { name: raw, exact: true }).click();
    await expect(dialog.locator("pre")).toContainText("# B12 真实场景验收");
    await page.keyboard.press("Escape");
    await page
      .getByRole("button", {
        name: english ? "Pinned summary" : "置顶摘要",
        exact: true,
      })
      .click();
    await page.locator('summary[title="/workspace/uploads"]').click();
    for (const folder of await page
      .locator('summary[title^="/workspace/uploads/"]')
      .all())
      await folder.click();
    await page
      .getByRole("button", {
        name: `${preview} b12-native-redis.csv`,
        exact: true,
      })
      .last()
      .click();
    dialog = page.getByRole("dialog", {
      name: "b12-native-redis.csv",
      exact: true,
    });
    await expect(dialog.locator("tbody tr")).toHaveCount(8);
    await page.keyboard.press("Escape");
    // Published output has its own immutable delivery metadata. It is not a
    // registered Workspace input row; assert each entry where it really exists.
    await page
      .getByRole("button", {
        name: `${preview} b12-native-redis.csv`,
        exact: true,
      })
      .last()
      .click();
    dialog = page.getByRole("dialog", {
      name: "b12-native-redis.csv",
      exact: true,
    });
    await expect(dialog.locator("tbody tr")).toHaveCount(8);
    expect(
      await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth),
    ).toBe(true);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    expect(modelCalls).toBe(0);
    await page.screenshot({
      path: info.outputPath("b12-real-workspace.png"),
      animations: "disabled",
    });
  });
}
