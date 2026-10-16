import { expect, type Locator, type Page } from "@playwright/test";

/** Capture actual visible pixels without locator.screenshot moving the iframe again. */
export async function captureTemplatePanel(
  page: Page,
  frame: Locator,
  subject: Locator,
  path: string,
) {
  await subject.evaluate((element) =>
    element.scrollIntoView({
      block: "start",
      inline: "nearest",
      behavior: "instant",
    }),
  );
  const stream = page.locator(".argus-chat__stream");
  let available = (await stream.boundingBox())!;
  const local = await subject.evaluate((element) => {
    const rect = element.getBoundingClientRect();
    return { x: rect.x, y: rect.y, width: rect.width, height: rect.height };
  });
  const first = (await frame.boundingBox())!;
  await stream.evaluate(
    (element, delta) => {
      // The document enables smooth scrolling. Capture must finish the scroll
      // synchronously before measuring pixels, including long real histories.
      element.scrollTo({ top: element.scrollTop + delta, behavior: "instant" });
    },
    first.y + local.y - available.y - 8,
  );
  await page.evaluate(
    () =>
      new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
      ),
  );
  available = (await stream.boundingBox())!;
  const outer = (await frame.boundingBox())!;
  const current = await subject.evaluate((element) => {
    const rect = element.getBoundingClientRect();
    const grid = element
      .closest(".argus-dashboard-grid")!
      .getBoundingClientRect();
    return {
      x: rect.x,
      y: rect.y,
      width: rect.width,
      height: rect.height,
      gridTop: grid.top,
      gridBottom: grid.bottom,
      viewportHeight: document.documentElement.clientHeight,
    };
  });
  expect(current.height).toBeGreaterThan(0);
  expect(current.width).toBeGreaterThan(0);
  expect(current.y).toBeGreaterThanOrEqual(Math.max(0, current.gridTop) - 1);
  expect(current.y + current.height).toBeLessThanOrEqual(
    Math.min(current.viewportHeight, current.gridBottom) + 1,
  );
  const clip = {
    x: outer.x + current.x,
    y: outer.y + current.y,
    width: current.width,
    height: current.height,
  };
  // A real trace/log table can be taller than the fixed desktop stream. Capture
  // all its pixels through natural scrolling, with the same bounds per tile.
  if (clip.height > available.height - 16) {
    const tileHeight = available.height - 16;
    const offsets = Array.from(
      { length: Math.ceil(clip.height / tileHeight) },
      (_, index) => Math.min(index * tileHeight, clip.height - tileHeight),
    );
    const tiles = [];
    for (const [index, offset] of offsets.entries()) {
      const box = (await frame.boundingBox())!;
      const rect = await subject.evaluate((element) => {
        const r = element.getBoundingClientRect();
        return { x: r.x, y: r.y, width: r.width, height: r.height };
      });
      await stream.evaluate(
        (element, delta) =>
          element.scrollTo({
            top: element.scrollTop + delta,
            behavior: "instant",
          }),
        box.y + rect.y + offset - available.y - 8,
      );
      await page.evaluate(
        () =>
          new Promise<void>((resolve) =>
            requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
          ),
      );
      const finalFrame = (await frame.boundingBox())!,
        finalStream = (await stream.boundingBox())!;
      const finalRect = await subject.evaluate((element) => {
        const r = element.getBoundingClientRect();
        return { x: r.x, y: r.y, width: r.width, height: r.height };
      });
      const tile = {
        x: finalFrame.x + finalRect.x,
        y: finalFrame.y + finalRect.y + offset,
        width: finalRect.width,
        height: Math.min(tileHeight, finalRect.height - offset),
      };
      expect(tile.y).toBeGreaterThanOrEqual(finalStream.y - 1);
      expect(tile.y + tile.height).toBeLessThanOrEqual(
        finalStream.y + finalStream.height + 1,
      );
      expect(tile.x).toBeGreaterThanOrEqual(finalStream.x - 1);
      expect(tile.x + tile.width).toBeLessThanOrEqual(
        finalStream.x + finalStream.width + 1,
      );
      expect(tile.height).toBeGreaterThan(0);
      const target =
        index === 0 ? path : path.replace(/\.png$/, `-part-${index + 1}.png`);
      await page.screenshot({
        path: target,
        clip: tile,
        animations: "disabled",
      });
      tiles.push({ offset, clip: tile, path: target });
    }
    return {
      clip: tiles[0]!.clip,
      frame: outer,
      stream: available,
      subject: current,
      tiles,
    };
  }
  expect(clip.y).toBeGreaterThanOrEqual(available.y - 1);
  expect(clip.y + clip.height).toBeLessThanOrEqual(
    available.y + available.height + 1,
  );
  expect(clip.x).toBeGreaterThanOrEqual(available.x - 1);
  expect(clip.x + clip.width).toBeLessThanOrEqual(
    available.x + available.width + 1,
  );
  await page.screenshot({ path, clip, animations: "disabled" });
  return { clip, frame: outer, stream: available, subject: current };
}
