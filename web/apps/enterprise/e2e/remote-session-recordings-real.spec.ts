import { expect, test } from "@playwright/test";
import { selectRowAction } from "./helpers/action-menu";
import {
  expectRecordingPlayerLayout,
  recordingPlayerViewports,
  recordingPresentation,
} from "./helpers/recording-player";

test.skip(
  process.env.ARGUS_RECORDING_PLAYER_REAL !== "1",
  "Requires an existing real session recording and a normal login",
);
test.use({
  trace: "off",
  video: "off",
  ignoreHTTPSErrors: false,
  reducedMotion: "reduce",
});

for (const locale of ["zh-CN", "en-US"])
  for (const theme of ["light", "dark"])
    for (const recording of [
      {
        key: "member-shell",
        reason: "B11b member shell zh-CN light readonly review",
      },
      {
        key: "bastion-ssh",
        reason: "B11b bastion ssh en-US dark readonly review",
      },
    ])
      test(`real recording ${recording.key} layout ${locale} ${theme}`, async ({
        page,
      }, info) => {
        test.setTimeout(90000);
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        await page.addInitScript(
          ({ locale, theme }) => {
            localStorage.setItem("argus.locale", locale);
            localStorage.setItem("argus.theme", theme);
          },
          { locale, theme },
        );
        await page.goto("/login");
        await page
          .locator('input[autocomplete="username"]')
          .fill(process.env.ARGUS_RECORDING_PLAYER_USER ?? "kakj");
        await page
          .locator('input[autocomplete="current-password"]')
          .fill(process.env.ARGUS_PLAN9_PASSWORD!);
        await page.locator('form button[type="submit"]').click();
        await expect(page).not.toHaveURL(/\/login/);
        try {
          await page.goto("/remote-sessions");
          await page
            .getByRole("tab", { name: /^(历史会话|Session History)$/ })
            .click();
          const row = page
            .locator("tbody tr")
            .filter({
              hasText:
                process.env.ARGUS_RECORDING_PLAYER_REASON ?? recording.reason,
            })
            .first();
          const response = page.waitForResponse((response) =>
            /\/remote-access-recordings\/[^/]+\/events/.test(response.url()),
          );
          await selectRowAction(row, /查看录像|View recording/);
          expect((await response).status()).toBe(200);
          const dialog = page.locator(".argus-recording-dialog");
          const player = dialog.locator(".argus-terminal-player");
          await expect(player.locator(".xterm-screen")).toBeVisible();
          const slider = player.getByRole("slider", {
            name: /录像进度|Recording position/,
          });
          const maximum = Number(await slider.getAttribute("max"));
          expect(maximum).toBeGreaterThan(0);
          await expect(player).toHaveAttribute("aria-busy", "false");
          // Let the dialog's entrance transform settle before measuring the
          // recording's physical font size.
          await expect
            .poll(async () => (await recordingPresentation(player)).fontSize)
            .toBeCloseTo(13);
          const checkpoints = [
            0,
            maximum * 0.8,
            maximum * 0.3,
            0,
            maximum * 0.8,
          ];
          for (const time of checkpoints) {
            await slider.fill(String(Math.floor(time * 10) / 10));
            await expect(player).toHaveAttribute("aria-busy", "false");
            const frame = await recordingPresentation(player);
            expect(frame.left).toBeCloseTo(0);
            expect(frame.top).toBeCloseTo(0);
            expect(frame.fontSize).toBeCloseTo(13);
          }
          await player
            .getByRole("button", { name: /^(从头播放|Restart)$/ })
            .click();
          // Play through the exact final timestamp: recorded durations are not
          // necessarily a multiple of the slider's 0.1s seek step.
          await player.getByRole("button", { name: "8x", exact: true }).click();
          await player.getByRole("button", { name: /^(播放|Play)$/ }).click();
          await expect(player.getByRole("status")).toHaveText(
            /回放结束|Replay ended/,
          );
          for (const viewport of recordingPlayerViewports) {
            await page.setViewportSize(viewport);
            await expectRecordingPlayerLayout(dialog);
            expect((await recordingPresentation(player)).fontSize).toBeCloseTo(
              13,
            );
            await page.screenshot({
              path: info.outputPath(`real-recording-${viewport.width}.png`),
            });
          }
          await player
            .getByRole("button", { name: /^(适应窗口|Fit)$/ })
            .click();
          await expect(player).toHaveAttribute("data-view-mode", "fit");
          const fitted = await recordingPresentation(player);
          for (const time of checkpoints) {
            await slider.fill(String(Math.floor(time * 10) / 10));
            await expect(player).toHaveAttribute("aria-busy", "false");
            await expectRecordingPlayerLayout(dialog);
            expect((await recordingPresentation(player)).scale).toBeCloseTo(
              fitted.scale,
              3,
            );
          }
          await player
            .getByRole("button", { name: /原始字号|Original font size/ })
            .click();
          expect((await recordingPresentation(player)).fontSize).toBeCloseTo(
            13,
          );
          await player
            .getByRole("button", { name: /^(从头播放|Restart)$/ })
            .click();
          expect(Number(await slider.inputValue())).toBe(0);
          await player.getByRole("button", { name: "1x", exact: true }).click();
          await player.getByRole("button", { name: /^(播放|Play)$/ }).click();
          await expect(
            player.getByRole("button", { name: /^(暂停|Pause)$/ }),
          ).toBeVisible();
          await expect
            .poll(async () => Number(await slider.inputValue()))
            .toBeGreaterThan(0);
          await player.getByRole("button", { name: /^(暂停|Pause)$/ }).click();
          await dialog
            .getByRole("tab", { name: /^(原始事件|Raw events)$/ })
            .click();
          await expect(dialog.locator(".argus-recording-events")).toContainText(
            "B11b-proof-start",
          );
          expect(errors).toEqual([]);
        } finally {
          await page.request.delete("/api/v1/enterprise/auth/session");
        }
      });
