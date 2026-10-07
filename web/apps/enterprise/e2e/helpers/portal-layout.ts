import { expect, type Locator, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
export async function assertNoSeriousAxe(page: Page) {
  // Test the resting appearance; HeroUI popovers briefly fade while entering.
  await page.evaluate(async () => {
    const animations = document
      .getAnimations()
      .filter((animation) =>
        Number.isFinite(animation.effect?.getComputedTiming().endTime),
      );
    await Promise.all(
      animations.map((animation) => animation.finished.catch(() => undefined)),
    );
  });
  const result = await new AxeBuilder({ page }).analyze();
  expect(
    result.violations.filter((v) => ["serious", "critical"].includes(v.impact)),
  ).toEqual([]);
}
export async function assertInboxContents(page: Page) {
  const items = page.locator(".argus-approval-item");
  await expect(items.first()).toBeVisible();
  const geometry = await items.evaluateAll((elements) =>
    elements.map((el) => {
      const box = el.getBoundingClientRect();
      const content = [
        ...el.querySelectorAll<HTMLElement>(
          ".argus-approval-item__top,.argus-approval-item__summary,.argus-approval-item__meta",
        ),
      ]
        .filter((e) => e.textContent?.trim())
        .map((e) => e.getBoundingClientRect());
      return {
        height: box.height,
        contained: content.every(
          (r) =>
            r.top >= box.top &&
            r.bottom <= box.bottom &&
            r.left >= box.left &&
            r.right <= box.right,
        ),
        ordered: content.every(
          (r, n) => n === 0 || r.top >= content[n - 1]!.bottom - 0.5,
        ),
      };
    }),
  );
  expect(geometry.every((g) => g.height > 32 && g.contained && g.ordered)).toBe(
    true,
  );
  const tabs = page.locator(".argus-approval-tabs").getByRole("tab");
  await expect(tabs).toHaveCount(2);
  for (const tab of await tabs.all()) await assertViewport(tab, page);
  const controls = page.locator(
    ".argus-approval-scope-tabs .argus-segments__control",
  );
  expect(
    await controls.evaluateAll((elements) =>
      elements.every((el) => el.getBoundingClientRect().height === 28),
    ),
  ).toBe(true);
  await expect(
    page.locator(
      ".argus-approval-scope-tabs .argus-segments__control[data-selected]",
    ),
  ).toHaveCount(1);
}
export async function assertFilterAlignment(page: Page) {
  const bar = page.locator(".argus-filter-bar").first();
  await expect(bar).toBeVisible();
  const rects = await bar
    .locator(
      ".argus-search-input,.argus-select__trigger,.argus-filter-bar__refresh",
    )
    .evaluateAll((elements) =>
      elements.map((el) => {
        const r = el.getBoundingClientRect();
        return { height: r.height, center: r.y + r.height / 2 };
      }),
    );
  expect(rects.length).toBeGreaterThanOrEqual(3);
  expect(
    rects.every(
      (r) => r.height === 32 && Math.abs(r.center - rects[0]!.center) < 1,
    ),
  ).toBe(true);
}
export async function assertViewport(locator: Locator, page: Page) {
  await expect(locator).toBeVisible();
  const rect = await locator.boundingBox();
  const viewport = page.viewportSize()!;
  expect(rect).not.toBeNull();
  expect(rect!.y).toBeGreaterThanOrEqual(48);
  expect(rect!.y + rect!.height).toBeLessThanOrEqual(viewport.height);
  expect(rect!.x).toBeGreaterThanOrEqual(0);
  expect(rect!.x + rect!.width).toBeLessThanOrEqual(viewport.width);
}
export async function assertEditorLayout(page: Page, english: boolean) {
  const tx = (zh: string, en: string) => (english ? en : zh);
  await expect(page.locator(".argus-panel-editor")).toBeVisible();
  const preview = await page
    .locator(".argus-panel-editor__preview")
    .boundingBox();
  const query = await page
    .locator(".argus-panel-editor__queries")
    .boundingBox();
  const rail = await page
    .locator(".argus-panel-editor__settings")
    .boundingBox();
  expect(rail!.x).toBeGreaterThan(preview!.x + preview!.width);
  expect(Math.abs(rail!.y - preview!.y)).toBeLessThan(1);
  expect(query!.y).toBeGreaterThan(preview!.y + preview!.height);
  const run = page.getByRole("button", {
    name: tx("运行查询", "Run query"),
    exact: true,
  });
  await assertViewport(run, page);
  expect((await run.boundingBox())!.height).toBe(32);
  await assertViewport(page.getByLabel(tx("统计图标题", "Panel title")), page);
  await assertViewport(
    page.getByRole("button", { name: tx("完成", "Done"), exact: true }),
    page,
  );
  for (const field of await page
    .locator(".argus-panel-editor__basics .argus-field")
    .all())
    await assertViewport(field, page);
  await assertViewport(
    page.getByRole("radiogroup", {
      name: tx("编辑来源", "Editing source"),
      exact: true,
    }),
    page,
  );
  const options = page.locator(".argus-chart-type-picker__option");
  expect(await options.count()).toBeLessThan(11);
  await expect(
    page.getByRole("button", {
      name: /^(?:全部 11 类图型|All 11 chart types)$/,
    }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: /^(?:全部 11 类图型|All 11 chart types)$/ })
    .click();
  await expect(options).toHaveCount(11);
  await page
    .getByRole("button", { name: /^(?:收起其他图型|Show fewer charts)$/ })
    .click();
}
