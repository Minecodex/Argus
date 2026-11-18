import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import { plan7Login } from "./helpers/plan7-login";

const enabled = process.env.ARGUS_B12_CLOSEOUT_REAL === "1";
const root = process.env.ARGUS_B12_RERUN_OUT ?? "";
const state = enabled
  ? JSON.parse(readFileSync(root + "/state.json", "utf8"))
  : {};
const verified = enabled
  ? JSON.parse(readFileSync(root + "/report-verification.json", "utf8"))
  : {};
test.skip(
  !enabled,
  "Requires the completed real B12 report; never sends model messages",
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
  [1920, true, "light"],
] as const) {
  test(`B12 verified report ${width} ${english ? "en" : "zh"} ${theme}`, async ({
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
    let modelRequests = 0;
    page.on("request", (request) => {
      if (
        request.method() === "POST" &&
        /\/(messages|preflight)$/.test(new URL(request.url()).pathname)
      )
        modelRequests++;
    });
    await plan7Login(
      page,
      "/login",
      "kakj",
      process.env.ARGUS_PLAN9_PASSWORD ?? "",
    );
    await page.goto("/?c=" + encodeURIComponent(state.conversation_id));
    const preview = english ? "Preview" : "预览";
    await page
      .getByRole("button", { name: `${preview} b12-closeout.md`, exact: true })
      .last()
      .click();
    const dialog = page.getByRole("dialog", {
      name: "b12-closeout.md",
      exact: true,
    });
    await expect(dialog).toBeVisible();
    await expect(
      dialog.getByRole("heading", {
        name: "B12 收尾复验报告（B12-Cache-Host 与 B12-Redis）",
        exact: true,
      }),
    ).toBeVisible();
    await expect(dialog.locator("table").first()).toBeVisible();
    const json = dialog.locator("pre").filter({ hasText: '"command_delta"' });
    await expect(json).toContainText('"command_delta": 37');
    await expect(json).toContainText('"peak_memory_bytes": 1562696');
    await expect(dialog).toContainText("ops/s");
    await expect(dialog).toContainText("53 点");
    await expect(dialog).toContainText("瞬时");
    await page.screenshot({
      path: info.outputPath("verified-report.png"),
      animations: "disabled",
    });
    await dialog
      .getByRole("button", { name: english ? "Raw" : "原文", exact: true })
      .click();
    await expect(dialog.locator("pre")).toContainText(
      "/workspace/queries/1e1215f6-8dfa-47ab-a313-7f4b03b67e8b/",
    );
    const download = await page.evaluate(async (path) => {
      const response = await fetch(path, { credentials: "same-origin" });
      const body = await response.arrayBuffer();
      const digest = await crypto.subtle.digest("SHA-256", body);
      return {
        status: response.status,
        bytes: body.byteLength,
        sha256: [...new Uint8Array(digest)]
          .map((n) => n.toString(16).padStart(2, "0"))
          .join(""),
      };
    }, `/api/v1/conversations/${state.conversation_id}/deliveries/${verified.artifact.id}/content`);
    expect(download.status).toBe(200);
    expect(download.bytes).toBe(verified.artifact.byte_size);
    expect(download.sha256).toBe(verified.artifact.content_hash);
    await page.keyboard.press("Escape");
    await page
      .getByRole("button", {
        name: `${preview} b12-native-redis.csv`,
        exact: true,
      })
      .first()
      .click();
    const csv = page.getByRole("dialog", {
      name: "b12-native-redis.csv",
      exact: true,
    });
    await expect(csv.locator("tbody tr")).toHaveCount(8);
    await expect(csv).toContainText("1562696");
    await page.keyboard.press("Escape");
    expect(modelRequests).toBe(0);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await info.attach("proof", {
      body: JSON.stringify({
        conversation: state.conversation_id,
        artifact: verified.artifact.id,
        hash: verified.artifact.content_hash,
        modelRequests,
        width,
        english,
        theme,
      }),
      contentType: "application/json",
    });
  });
}
