import type { Page } from "@playwright/test";
import fs from "node:fs/promises";
import { execFileSync } from "node:child_process";
import path from "node:path";

export type RegionBox = { x: number; y: number; width: number; height: number };

/** Preserve layout and assemble real horizontal/vertical scroll exposures. */
export async function captureRegion(
  page: Page,
  root: string,
  box: RegionBox,
  file: string,
  repository: string,
): Promise<RegionBox> {
  const locator = page.locator(root).first();
  const initial = (await locator.boundingBox())!;
  const hosts = await locator.evaluate((element) => {
    return (["x", "y"] as const).map((axis) => {
      for (let node = element.parentElement; node; node = node.parentElement) {
        const style = getComputedStyle(node),
          horizontal = axis === "x";
        if (
          /auto|scroll/.test(horizontal ? style.overflowX : style.overflowY) &&
          (horizontal
            ? node.scrollWidth > node.clientWidth
            : node.scrollHeight > node.clientHeight)
        ) {
          node.setAttribute(`data-plan6-scroll-${axis}`, "true");
          const r = node.getBoundingClientRect();
          return {
            axis,
            selector: `[data-plan6-scroll-${axis}="true"]`,
            position: horizontal ? node.scrollLeft : node.scrollTop,
            start: horizontal
              ? Math.max(0, r.x + node.clientLeft)
              : Math.max(0, r.y + node.clientTop),
            end: horizontal
              ? Math.min(innerWidth, r.x + node.clientLeft + node.clientWidth)
              : Math.min(innerHeight, r.y + node.clientTop + node.clientHeight),
          };
        }
      }
      return {
        axis,
        selector: "",
        position: axis === "x" ? scrollX : scrollY,
        start: 0,
        end: axis === "x" ? innerWidth : innerHeight,
      };
    });
  });
  const [xHost, yHost] = hosts;
  const offset = { x: box.x - initial.x, y: box.y - initial.y };
  // Use the same pixel boundaries as the report's crop. Rounding a fractional
  // height independently can invent an extra pixel the scroll host cannot expose.
  const width = Math.round(box.x + box.width) - Math.round(box.x),
    height = Math.round(box.y + box.height) - Math.round(box.y);
  const pieces = [];
  const move = async (x: number, y: number) => {
    await page.evaluate(
      ({ hosts, x, y }) => {
        for (const host of hosts) {
          const position = host.axis === "x" ? x : y;
          const node = host.selector
            ? document.querySelector(host.selector)!
            : window;
          node.scrollTo(
            host.axis === "x" ? { left: position } : { top: position },
          );
        }
        return new Promise<void>((done) =>
          requestAnimationFrame(() => requestAnimationFrame(() => done())),
        );
      },
      { hosts, x, y },
    );
  };
  await page.mouse.move(0, 0);
  try {
    let coveredY = 0;
    for (let row = 0; coveredY < height; row++) {
      if (row > 30)
        throw new Error("vertical capture did not expose all business content");
      let coveredX = 0,
        nextY = height;
      for (let column = 0; coveredX < width; column++) {
        if (column > 30)
          throw new Error(
            "horizontal capture did not expose all business content",
          );
        await move(
          Math.max(0, xHost.position + box.x - xHost.start + coveredX),
          Math.max(0, yHost.position + box.y - yHost.start + coveredY),
        );
        const current = await locator.boundingBox();
        if (!current)
          throw new Error("business region disappeared during capture");
        const shifted = { x: current.x + offset.x, y: current.y + offset.y };
        const left = Math.max(shifted.x, xHost.start),
          top = Math.max(shifted.y, yHost.start);
        const right = Math.min(shifted.x + box.width, xHost.end),
          bottom = Math.min(shifted.y + box.height, yHost.end);
        const x = Math.round(left) - Math.round(shifted.x),
          y = Math.round(top) - Math.round(shifted.y);
        const w = Math.min(width - x, Math.round(right) - Math.round(left)),
          h = Math.min(height - y, Math.round(bottom) - Math.round(top));
        if (w <= 0 || h <= 0 || x > coveredX || y > coveredY)
          throw new Error(
            `scroll capture has a missing business tile: ${JSON.stringify({ root, row, column, box, initial, shifted, xHost, yHost, x, y, w, h, coveredX, coveredY })}`,
          );
        const piece = file.replace(/\.png$/, `-scroll-${row}-${column}.png`);
        await page.screenshot({ path: piece, animations: "disabled" });
        pieces.push({
          file: piece,
          source: {
            x: Math.round(left),
            y: Math.round(top),
            width: w,
            height: h,
          },
          x,
          y,
        });
        if (x + w <= coveredX)
          throw new Error("scroll host cannot expose the remaining columns");
        coveredX = x + w;
        nextY = Math.min(nextY, y + h);
      }
      if (nextY <= coveredY)
        throw new Error("scroll host cannot expose the remaining rows");
      coveredY = nextY;
    }
  } finally {
    await move(xHost.position, yHost.position);
    await page.evaluate((hosts) => {
      for (const host of hosts)
        if (host.selector)
          document
            .querySelector(host.selector)
            ?.removeAttribute(`data-plan6-scroll-${host.axis}`);
    }, hosts);
  }
  const manifest = file.replace(/\.png$/, "-scroll.json");
  await fs.writeFile(
    manifest,
    JSON.stringify({ width, height, pieces, output: file }),
  );
  execFileSync("python", [
    path.join(repository, "scripts/plan6-capture-stitch.py"),
    manifest,
  ]);
  return { x: 0, y: 0, width, height };
}
