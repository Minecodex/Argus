import { expect, test } from "@playwright/test";
import { plan7Login } from "./helpers/plan7-login";

const conversation = process.env.ARGUS_B12_RERUN_CONVERSATION ?? "";
const report = process.env.ARGUS_B12_RERUN_REPORT ?? "";
test.skip(
  process.env.ARGUS_B12_RERUN_REAL !== "1",
  "Read-only inspection of a completed real B12 rerun; no paid model requests",
);
test.use({
  ignoreHTTPSErrors: false,
  trace: "off",
  video: "off",
  reducedMotion: "reduce",
});

for (const [width, english, theme] of [
  [1280, false, "light"],
  [1440, false, "dark"],
  [1440, true, "light"],
  [1920, true, "dark"],
] as const) {
  test(`real B12 rerun ${width} ${english ? "en" : "zh"} ${theme}`, async ({
    page,
  }, info) => {
    await page.setViewportSize({ width, height: 1000 });
    await page.addInitScript(
      ({ english, theme }) => {
        localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
        localStorage.setItem("argus.theme", theme);
      },
      { english, theme },
    );
    let modelRequests = 0;
    page.on("request", (request) => {
      if (
        request.method() === "POST" &&
        /\/(messages|preflight)$/.test(new URL(request.url()).pathname)
      ) modelRequests++;
    });
    await plan7Login(page, "/login", "kakj", process.env.ARGUS_PLAN9_PASSWORD ?? "");
    await page.goto("/?c=" + encodeURIComponent(conversation));
    const preview = english ? "Preview" : "预览";
    await page.getByRole("button", {
      name: `${preview} b12-native-redis.csv`, exact: true,
    }).first().click();
    let dialog = page.getByRole("dialog", { name: "b12-native-redis.csv", exact: true });
    await expect(dialog.locator("tbody tr")).toHaveCount(8);
    await expect(dialog.getByRole("table")).toContainText("1562696");
    await page.keyboard.press("Escape");
    await page.getByRole("button", { name: `${preview} ${report}`, exact: true }).last().click();
    dialog = page.getByRole("dialog", { name: report, exact: true });
    await expect(dialog.locator(".argus-markdown")).toBeVisible();
    await expect(dialog).toContainText("command_delta");
    await expect(dialog).toContainText("1562696");
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: info.outputPath("real-rerun-report.png"), animations: "disabled" });
    expect(modelRequests).toBe(0);
  });
}
