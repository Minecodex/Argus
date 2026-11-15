import { expect, test, type Page } from "@playwright/test";
import { selectRowAction } from "./helpers/action-menu";
import {
  expectRecordingPlayerLayout,
  recordingPlayerViewports,
  recordingPresentation,
} from "./helpers/recording-player";

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    if (!window.localStorage.getItem("argus.locale")) {
      window.localStorage.setItem("argus.locale", "zh-CN");
    }
    if (!window.localStorage.getItem("argus.theme")) {
      window.localStorage.setItem("argus.theme", "dark");
    }
  });
});

async function login(page: Page, username = "root") {
  await page.goto("/login");
  await page.locator('input[autocomplete="username"]').fill(username);
  await page.locator('input[autocomplete="current-password"]').fill("123456");
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/\/login/);
}

test("recordings: wide detail dialog replays the session and exposes raw events", async ({
  page,
}) => {
  await login(page);
  await page.goto("/remote-sessions");
  await page.getByRole("tab", { name: "会话录像" }).click();

  // mock 种子录像（available）出现在列表中。
  const row = page.getByRole("row", { name: /ras-seed-1/ });
  await expect(row).toBeVisible();
  await selectRowAction(row, "查看录像");

  // 详情弹框为宽版（--lg），不再被 520px 默认宽度裁剪。
  const dialog = page.locator(".argus-dialog--lg");
  await expect(dialog).toBeVisible();
  const box = await dialog.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.width).toBeGreaterThan(700);

  // 事件自动翻页拉全后进入回放视图：终端画面 + 视频式控制条。
  const player = dialog.locator(".argus-terminal-player");
  await expect(player).toBeVisible({ timeout: 15_000 });
  const scrollbar = player.locator(
    ".xterm-scrollable-element > .scrollbar.vertical",
  );
  await expect(scrollbar).toHaveCount(1);
  expect(
    await scrollbar.evaluate((element) => getComputedStyle(element).width),
  ).toBe("9px");
  expect(
    await scrollbar
      .locator(".slider")
      .evaluate((element) => getComputedStyle(element).width),
  ).toBe("9px");
  await expect(player.getByRole("slider", { name: "录像进度" })).toBeVisible();
  await expect(player.getByText(/^00:00 \/ 00:10$/)).toBeVisible();
  await expect(player.getByRole("button", { name: "4x" })).toBeVisible();

  // 8 倍速播放 10.4s 录像约 1.3s 到达终点并自动停止。
  await player.getByRole("button", { name: "8x" }).click();
  await player.getByRole("button", { name: "播放", exact: true }).click();
  await expect(player.getByText("回放结束")).toBeVisible({ timeout: 10_000 });
  await expect(player.getByText(/^00:10 \/ 00:10$/)).toBeVisible();

  // 从头重放可用；拖动进度条 seek 到中间位置。
  await player.getByRole("button", { name: "从头播放" }).click();
  await expect(player.getByText(/^00:00 \/ 00:10$/)).toBeVisible();
  await player.getByRole("slider", { name: "录像进度" }).fill("5");
  await expect(player.getByText(/^00:05 \/ 00:10$/)).toBeVisible();

  // 原始事件视图保留审计用的明文事件流。
  await dialog.getByRole("tab", { name: "原始事件" }).click();
  await expect(dialog.locator(".argus-recording-events")).toContainText(
    "uptime",
  );
  await expect(dialog.locator(".argus-recording-events")).toContainText(
    "120x30",
  );
});

for (const locale of ["zh-CN", "en-US"])
  for (const theme of ["light", "dark"])
    test(`recording layout ${locale} ${theme}: bounded canvas, aligned controls and keyboard`, async ({
      page,
    }, info) => {
      await page.addInitScript(
        ({ locale, theme }) => {
          localStorage.setItem("argus.locale", locale);
          localStorage.setItem("argus.theme", theme);
        },
        { locale, theme },
      );
      await login(page);
      // Vary both dimensions sharply. Neither native size nor fit mode may
      // recenter or change text size as the recording crosses these boundaries.
      await page.evaluate(() => {
        const key = Object.keys(localStorage).find((key) =>
          key.startsWith("argus-mock:db-"),
        )!;
        const db = JSON.parse(localStorage.getItem(key)!);
        db.remoteAccessRecordingEvents["rar-seed-1"] = [
          { time: 0, type: "r", data: "80x24" },
          { time: 0, type: "o", data: "\u001b[2J\u001b[HFRAME-0" },
          { time: 1, type: "r", data: "320x90" },
          { time: 1, type: "o", data: "\u001b[2J\u001b[HFRAME-1" },
          { time: 2, type: "r", data: "40x8" },
          { time: 2, type: "o", data: "\u001b[2J\u001b[HFRAME-2" },
          { time: 3, type: "r", data: "180x30" },
          { time: 3, type: "o", data: "\u001b[2J\u001b[HFRAME-3" },
          { time: 11, type: "o", data: "\r\nREPLAY-END" },
        ];
        localStorage.setItem(key, JSON.stringify(db));
      });
      await page.goto("/remote-sessions");
      await page.getByRole("tab", { name: /^(会话录像|Recordings)$/ }).click();
      await selectRowAction(
        page.getByRole("row", { name: /ras-seed-1/ }),
        /查看录像|View recording/,
      );
      const dialog = page.locator(".argus-recording-dialog");
      const player = dialog.locator(".argus-terminal-player");
      await expect(player.locator(".xterm-screen")).toBeVisible();
      const slider = player.getByRole("slider", {
        name: /录像进度|Recording position/,
      });
      const checkpoints = [
        [0, "FRAME-0"],
        [1.5, "FRAME-1"],
        [2.5, "FRAME-2"],
        [3.5, "FRAME-3"],
        [1.5, "FRAME-1"],
        [0, "FRAME-0"],
      ] as const;
      const assertFrame = async (time: number, marker: string) => {
        await slider.fill(String(time));
        await expect(player).toHaveAttribute("aria-busy", "false");
        await expect(player.locator(".xterm-rows")).toContainText(marker);
      };
      for (const [time, marker] of checkpoints) {
        await assertFrame(time, marker);
        const frame = await recordingPresentation(player);
        expect(frame.left).toBeCloseTo(0);
        expect(frame.top).toBeCloseTo(0);
        expect(frame.fontSize).toBeCloseTo(13);
        expect(frame.scale).toBeCloseTo(1);
      }
      for (const viewport of recordingPlayerViewports) {
        await page.setViewportSize(viewport);
        await expectRecordingPlayerLayout(dialog);
        expect((await recordingPresentation(player)).fontSize).toBeCloseTo(13);
        await page.screenshot({
          path: info.outputPath(`recording-${viewport.width}.png`),
        });
      }
      await player.getByRole("button", { name: /^(适应窗口|Fit)$/ }).click();
      await expect(player).toHaveAttribute("data-view-mode", "fit");
      const fitted = await recordingPresentation(player);
      for (const [time, marker] of checkpoints) {
        await assertFrame(time, marker);
        await expectRecordingPlayerLayout(dialog);
        expect((await recordingPresentation(player)).scale).toBeCloseTo(
          fitted.scale,
          3,
        );
      }
      await player
        .getByRole("button", { name: /原始字号|Original font size/ })
        .click();
      expect((await recordingPresentation(player)).fontSize).toBeCloseTo(13);
      await assertFrame(1.5, "FRAME-1");
      await player
        .locator(".argus-terminal-player__viewport")
        .evaluate((element) => element.scrollTo(120, 80));
      await assertFrame(1.6, "FRAME-1");
      expect((await recordingPresentation(player)).scrollLeft).toBe(120);
      expect((await recordingPresentation(player)).scrollTop).toBe(80);
      await player
        .getByRole("button", { name: /^(从头播放|Restart)$/ })
        .click();
      expect((await recordingPresentation(player)).scrollLeft).toBe(0);
      expect((await recordingPresentation(player)).scrollTop).toBe(0);
      await slider.fill("5");
      await slider.focus();
      await page.keyboard.press("ArrowRight");
      expect(Number(await slider.inputValue())).toBeCloseTo(5.1, 1);
      await player
        .getByRole("button", { name: /^(从头播放|Restart)$/ })
        .click();
      const play = player.getByRole("button", { name: /^(播放|Play)$/ });
      await play.focus();
      await page.keyboard.press("Space");
      await expect(
        player.getByRole("button", { name: /^(暂停|Pause)$/ }),
      ).toBeVisible();
    });
