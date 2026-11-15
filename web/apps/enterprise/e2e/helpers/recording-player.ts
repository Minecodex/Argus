import { expect, type Locator } from "@playwright/test";

export async function recordingPresentation(player: Locator) {
  return player.evaluate((element) => {
    const viewport = element.querySelector<HTMLElement>(
      ".argus-terminal-player__viewport",
    )!;
    const screen = element.querySelector<HTMLElement>(".xterm-screen")!;
    const content = element.querySelector<HTMLElement>(".xterm-rows") ?? screen;
    const box = screen.getBoundingClientRect(),
      view = viewport.getBoundingClientRect();
    // offsetWidth rounds fractional cell widths; use the untransformed CSS
    // width so a native 13px font is not reported as 13.02px.
    const scale = box.width / parseFloat(getComputedStyle(screen).width);
    return {
      mode: element.getAttribute("data-view-mode"),
      left: box.left - view.left + viewport.scrollLeft,
      top: box.top - view.top + viewport.scrollTop,
      right: box.right - view.right,
      bottom: box.bottom - view.bottom,
      scale,
      fontSize: parseFloat(getComputedStyle(content).fontSize) * scale,
      scrollLeft: viewport.scrollLeft,
      scrollTop: viewport.scrollTop,
    };
  });
}

/** Check visible geometry, including a recording wider/taller than the dialog. */
export async function expectRecordingPlayerLayout(dialog: Locator) {
  const player = dialog.locator(".argus-terminal-player");
  await expect(player.locator(".xterm-screen")).toBeVisible();
  await expect
    .poll(() =>
      dialog.evaluate((element) => {
        const body = element.querySelector(".argus-dialog__body")!;
        return {
          horizontal: Math.max(0, body.scrollWidth - body.clientWidth - 1),
          vertical: Math.max(0, body.scrollHeight - body.clientHeight - 1),
        };
      }),
    )
    .toEqual({ horizontal: 0, vertical: 0 });
  await expect
    .poll(async () => {
      const frame = await recordingPresentation(player);
      return (
        Math.abs(frame.left) < 1 &&
        Math.abs(frame.top) < 1 &&
        (frame.mode === "native"
          ? Math.abs(frame.scale - 1) < 0.001
          : frame.right <= 1 && frame.bottom <= 1)
      );
    })
    .toBe(true);
  const controls = player.locator(".argus-terminal-player__controls");
  const geometry = await controls.evaluate((element) => {
    const bounds = element.getBoundingClientRect();
    const children = [...element.children].map((child) =>
      child.getBoundingClientRect(),
    );
    const buttons = [...element.querySelectorAll("button")].map((button) => {
      const box = button.getBoundingClientRect();
      const icon = button.querySelector("svg")?.getBoundingClientRect();
      return icon
        ? Math.abs(box.y + box.height / 2 - icon.y - icon.height / 2)
        : 0;
    });
    return {
      overflow: element.scrollWidth - element.clientWidth,
      contained: children.every(
        (box) => box.left >= bounds.left && box.right <= bounds.right + 1,
      ),
      centers: [
        ...children,
        element
          .querySelector(".argus-range-slider__thumb")!
          .getBoundingClientRect(),
      ].map((box) => box.y + box.height / 2),
      icons: buttons,
    };
  });
  expect(geometry.overflow).toBeLessThanOrEqual(1);
  expect(geometry.contained).toBe(true);
  expect(
    Math.max(...geometry.centers) - Math.min(...geometry.centers),
  ).toBeLessThanOrEqual(1);
  expect(Math.max(...geometry.icons)).toBeLessThanOrEqual(1);
  const restart = player.getByRole("button", { name: /^(从头播放|Restart)$/ });
  const contrast = await restart.evaluate((element) => {
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = 1;
    const context = canvas.getContext("2d")!;
    const rgba = (color: string) => {
      context.clearRect(0, 0, 1, 1);
      context.fillStyle = color;
      context.fillRect(0, 0, 1, 1);
      return [...context.getImageData(0, 0, 1, 1).data];
    };
    let background = [255, 255, 255];
    const ancestors = [];
    for (let node: Element | null = element; node; node = node.parentElement)
      ancestors.unshift(node);
    for (const node of ancestors) {
      const [r, g, b, a] = rgba(getComputedStyle(node).backgroundColor);
      background = [r, g, b].map(
        (value, i) => (value * a) / 255 + background[i] * (1 - a / 255),
      );
    }
    const foreground = rgba(
      getComputedStyle(element.querySelector("svg")!).color,
    );
    const luminance = (rgb: number[]) =>
      rgb.slice(0, 3).reduce((sum, value, i) => {
        const channel = value / 255;
        return (
          sum +
          (channel <= 0.04045
            ? channel / 12.92
            : ((channel + 0.055) / 1.055) ** 2.4) *
            [0.2126, 0.7152, 0.0722][i]
        );
      }, 0);
    const a = luminance(foreground),
      b = luminance(background);
    return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
  });
  expect(contrast).toBeGreaterThanOrEqual(3);
}

export const recordingPlayerViewports = [
  { width: 1280, height: 720 },
  { width: 1440, height: 900 },
  { width: 1920, height: 1080 },
];
