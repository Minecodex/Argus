import { expect, test, type Page } from "@playwright/test";
import type { DashboardDraft, DashboardPanel } from "@argus/api-client";
import { dashboardsEn, dashboardsZh } from "../src/i18n/dashboards";
import { createMfaLogin } from "./helpers/mfa-login";
import { dashboardAPI } from "./helpers/planv2-api";

const login = createMfaLogin("enterprise");
test.use({ viewport: { width: 1440, height: 1100 } });
for (const english of [false, true]) {
  test(`real grid collision, minimum size, cancellation and recovery ${english ? "en dark" : "zh light"}`, async ({
    page,
    context,
  }, info) => {
    test.skip(
      process.env.ARGUS_PLANV2_E2E !== "1",
      "Requires PlanV2 Kubernetes environment",
    );
    test.setTimeout(180000);
    const d = (english ? dashboardsEn : dashboardsZh).dashboards;
    await context.addInitScript(
      ({ english }) => {
        localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
        localStorage.setItem("argus.theme", english ? "dark" : "light");
      },
      { english },
    );
    await login(
      page,
      "/login",
      process.env.ARGUS_PLANV2_USERNAME!,
      process.env.ARGUS_PLANV2_PASSWORD!,
    );
    await page.goto("/dashboards");
    const gallery = await (
      await dashboardAPI(
        page,
        `/dashboards/${process.env.ARGUS_PLANV2_GALLERY_ID}`,
      )
    ).json();
    const base: DashboardPanel = gallery.revision.spec.panels.find(
      (p: DashboardPanel) => p.type === "stat",
    );
    const panels = ["a", "b", "c", "d"].map((id, i) => ({
      ...base,
      id,
      title: `Grid ${id}`,
      layout: {
        x: (i % 2) * 6,
        y: Math.floor(i / 2) * 24,
        w: 6,
        h: 24,
        min_w: 3,
        min_h: 12,
      },
    }));
    const draft: DashboardDraft = await (
      await dashboardAPI(
        page,
        "/dashboard-drafts",
        "POST",
        {
          name: `Real layout ${english ? "en" : "zh"}`,
          description: "",
          spec: { ...gallery.revision.spec, panels, variables: [] },
          proposed_bindings: [],
        },
        201,
      )
    ).json();
    const path = `/dashboard-drafts/${draft.id}`;
    await page.goto(path);
    const resize = page.getByRole("button", {
      name: `${english ? "Resize panel" : "调整统计图尺寸"} Grid a`,
      exact: true,
    });
    await expect(resize).toBeVisible();
    const grid = await page.locator(".argus-dashboard-grid").boundingBox();
    const rowHeight = draft.spec.layout.row_height;
    // Grow across both neighbors, including the second row.
    const saved = saveResponse(page, draft.id);
    await drag(page, resize, (grid!.width / 12) * 3, rowHeight * 12);
    const expanded: DashboardDraft = await (await saved).json();
    expect(
      expanded.spec.panels.find((p) => p.id === "a")!.layout,
    ).toMatchObject({ w: 9, h: 36 });
    noOverlaps(expanded.spec.panels);
    expect(expanded.spec.panels.find((p) => p.id === "b")!.layout.y).toBe(36);
    expect(expanded.spec.panels.find((p) => p.id === "c")!.layout.y).toBe(36);
    expect(expanded.spec.panels.find((p) => p.id === "d")!.layout.y).toBe(60);
    // Clamp both dimensions at their declared minima with a real pointer gesture.
    const shrunk = saveResponse(page, draft.id);
    await drag(page, resize, -grid!.width, -rowHeight * 100);
    const minimum: DashboardDraft = await (await shrunk).json();
    expect(minimum.spec.panels.find((p) => p.id === "a")!.layout).toMatchObject(
      { w: 3, h: 12 },
    );
    noOverlaps(minimum.spec.panels);
    // A pointer drag must take keyboard focus so Escape cancels this gesture.
    await page.getByRole("button", { name: d.metadata, exact: true }).focus();
    const move = page.getByRole("button", {
      name: `${english ? "Move panel" : "移动统计图"} Grid a`,
      exact: true,
    });
    const box = (await move.boundingBox())!;
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await page.mouse.down();
    await page.mouse.move(
      box.x + box.width / 2 + grid!.width / 4,
      box.y + box.height / 2 + rowHeight * 5,
      { steps: 8 },
    );
    await page.keyboard.press("Escape");
    await page.mouse.up();
    await expect(page.locator('[data-panel-id="a"]')).toHaveCSS(
      "grid-column-start",
      "1",
    );
    await expect(page.locator('[data-panel-id="a"]')).toHaveCSS(
      "grid-row-start",
      "1",
    );
    // Synchronize through the save command, then check the authoritative draft.
    await page.getByRole("button", { name: d.save, exact: true }).click();
    const restored = await (await dashboardAPI(page, path)).json();
    expect(restored.spec.panels).toEqual(minimum.spec.panels);
    await page.reload();
    await expect(page.locator('[data-panel-id="a"]')).toHaveCSS(
      "grid-column-end",
      "span 3",
    );
    await expect(page.locator('[data-panel-id="d"]')).toHaveCSS(
      "grid-row-start",
      "61",
    );
    const keyboard = saveResponse(page, draft.id);
    await move.press("Shift+ArrowRight");
    expect(
      (await (await keyboard).json()).spec.panels.find(
        (p: DashboardPanel) => p.id === "a",
      ).layout.w,
    ).toBe(4);
    await page.screenshot({
      path: info.outputPath("grid-boundaries.png"),
      fullPage: true,
    });
  });
}

function saveResponse(page: Page, id: string) {
  return page.waitForResponse(
    (r) =>
      r.request().method() === "PATCH" &&
      r.url().endsWith(`/dashboard-drafts/${id}`) &&
      r.status() === 200,
  );
}
async function drag(
  page: Page,
  handle: ReturnType<Page["getByRole"]>,
  dx: number,
  dy: number,
) {
  await handle.scrollIntoViewIfNeeded();
  const box = (await handle.boundingBox())!;
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(
    box.x + box.width / 2 + dx,
    box.y + box.height / 2 + dy,
    { steps: 10 },
  );
  await page.mouse.up();
}
function noOverlaps(panels: DashboardPanel[]) {
  for (const [i, a] of panels.entries())
    for (const b of panels.slice(i + 1)) {
      const r = a.layout,
        s = b.layout;
      expect(
        r.x < s.x + s.w &&
          s.x < r.x + r.w &&
          r.y < s.y + s.h &&
          s.y < r.y + r.h,
      ).toBe(false);
    }
}
