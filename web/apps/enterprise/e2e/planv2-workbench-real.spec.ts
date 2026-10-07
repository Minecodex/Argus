import { expect, test, type Locator, type Page } from "@playwright/test";
import type { DashboardDraft } from "@argus/api-client";
import { dashboardsEn, dashboardsZh } from "../src/i18n/dashboards";
import { createMfaLogin } from "./helpers/mfa-login";
import { selectTrigger } from "./helpers/select";

test.use({ actionTimeout: 15000, navigationTimeout: 30000 });
const login = createMfaLogin("enterprise");
for (const english of [false, true]) {
  const d = (english ? dashboardsEn : dashboardsZh).dashboards;
  test(`real workbench creates three signals, resolves stale tabs and restores archived drafts ${english ? "en dark" : "zh light"}`, async ({
    page,
    context,
  }, testInfo) => {
    test.skip(
      process.env.ARGUS_PLANV2_E2E !== "1",
      "Requires the PlanV2 Kubernetes suite",
    );
    test.setTimeout(240000);
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
      process.env.ARGUS_PLANV2_USERNAME ?? "",
      process.env.ARGUS_PLANV2_PASSWORD ?? "",
    );
    await page.goto("/dashboards");
    await button(page, d.create).click();
    const create = page.getByRole("dialog", { name: d.create, exact: true });
    const title = `Real three signals ${english ? "English" : "Chinese"}`;
    await create
      .getByRole("textbox", { name: d.name, exact: true })
      .fill(title);
    await button(create, d.done).click();
    await expect(page).toHaveURL(/dashboard-drafts/);

    for (const signal of ["metrics", "logs", "traces"]) {
      await button(page, d.addPanel).first().click();
      const preset =
        signal === "metrics" ? "custom" : signal === "logs" ? "logs" : "traces";
      await button(
        page.getByRole("dialog", { name: d.addPanel, exact: true }),
        d.editor.presets[preset],
      ).click();
      await expect(page).toHaveURL(/\/panels\//);
      const editor = page.locator(".argus-panel-editor");
      await editor.getByLabel(d.panelTitle).fill(`Observed ${signal}`);
      if (signal === "metrics") {
        await select(page, editor, d.source, d.editor.sources.otlp);
        await editor
          .getByRole("combobox", { name: d.metric, exact: true })
          .fill("argus_m7_e2e_gauge_planv2");
        const converted = page.waitForResponse((r) =>
          r.url().includes("/dashboard-spec/convert-panel"),
        );
        await select(page, editor, d.mode, d.dsl);
        expect((await converted).status()).toBe(200);
        await expect(
          editor.getByRole("textbox", {
            name: english ? "Query expression" : "查询语句",
            exact: true,
          }),
        ).toHaveValue(/^argus_m7_e2e_gauge_planv2(?:\{\})?$/);
        await select(page, editor, d.mode, d.builder);
        await expect(
          editor.getByRole("combobox", { name: d.metric, exact: true }),
        ).toHaveValue("argus_m7_e2e_gauge_planv2");
      }
      await button(editor, d.done).click();
      await expect(page).toHaveURL(/\/dashboard-drafts\/[^/]+$/);
      await expect(page.getByText(d.saved, { exact: false })).toBeVisible();
    }
    const resized = saveResponse(page);
    await button(
      page,
      `${english ? "Resize panel" : "调整统计图尺寸"} Observed metrics`,
    ).press("ArrowRight");
    const resizedDraft: DashboardDraft = await (await resized).json();
    const metric = resizedDraft.spec.panels.find(
      (p) => p.title === "Observed metrics",
    )!;
    expect(metric.layout.w).toBe(7);
    const taller = saveResponse(page);
    await button(
      page,
      `${english ? "Resize panel" : "调整统计图尺寸"} Observed metrics`,
    ).press("ArrowDown");
    const heightDraft: DashboardDraft = await (await taller).json();
    expect(
      heightDraft.spec.panels.find((p) => p.id === metric.id)!.layout.h,
    ).toBe(metric.layout.h + 1);
    const handle = button(
      page,
      `${english ? "Move panel" : "移动统计图"} Observed metrics`,
    );
    await handle.scrollIntoViewIfNeeded();
    const box = (await handle.boundingBox())!;
    const grid = (await page.locator(".argus-dashboard-grid").boundingBox())!;
    const moved = saveResponse(page);
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await page.mouse.down();
    await page.mouse.move(
      box.x + box.width / 2 + grid.width / 12,
      box.y + box.height / 2,
      { steps: 6 },
    );
    await page.mouse.up();
    const movedDraft: DashboardDraft = await (await moved).json();
    expect(
      movedDraft.spec.panels.find((p) => p.id === metric.id)!.layout.x,
    ).toBe(1);
    await page.reload();
    await expect(page.locator(`[data-panel-id="${metric.id}"]`)).toHaveCSS(
      "grid-column-start",
      "2",
    );
    await expect(page.locator(`[data-panel-id="${metric.id}"]`)).toHaveCSS(
      "grid-column-end",
      "span 7",
    );
    const sampled = page.waitForResponse((r) => r.url().endsWith("/sample"));
    await button(page, d.sampleRun).click();
    const sample = await (await sampled).json();
    expect(sample.validation.valid).toBe(true);
    expect(sample.sample.execution.panels).toHaveLength(3);
    expect(
      sample.sample.execution.panels.every(
        (p: { status: string }) => p.status === "success",
      ),
    ).toBe(true);
    await publish(page);
    const publishedURL = page.url();
    await button(page, d.edit).click();
    await expect(page).toHaveURL(/dashboard-drafts/);
    const draftURL = page.url();
    const stale = await context.newPage();
    await stale.goto(draftURL);
    await expect(button(stale, d.metadata)).toBeVisible();
    await description(page, "First tab wins", 200);
    await description(stale, "Stale tab must not overwrite", 409);
    const conflict = stale.getByRole("dialog", {
      name: d.conflict,
      exact: true,
    });
    await expect(conflict).toContainText("First tab wins");
    await expect(conflict).toContainText("Stale tab must not overwrite");
    await button(conflict, d.useServer).click();
    await expect(conflict).not.toBeVisible();
    await button(stale, d.metadata).click();
    await expect(stale.getByLabel(d.descriptionField)).toHaveValue(
      "First tab wins",
    );
    await stale.keyboard.press("Escape");
    const reader = await context.newPage();
    await reader.goto(publishedURL);
    await expect(
      reader.getByRole("heading", { name: title, exact: true }),
    ).toBeVisible();
    await expect(
      reader.getByText("First tab wins", { exact: false }),
    ).toHaveCount(0);
    await publish(page);
    await stale.close();

    await button(page, d.edit).click();
    await expect(page).toHaveURL(/dashboard-drafts/);
    await description(page, "Unpublished draft survives archive", 200);
    await reader.goto("/dashboards");
    const card = reader.locator(".argus-resource-card").filter({
      has: reader.getByRole("heading", { name: title, exact: true }),
    });
    await button(card, english ? "More actions" : "更多操作").click();
    await reader
      .getByRole("menuitem", { name: d.archive, exact: true })
      .click();
    await change(reader);
    await reader.getByRole("button", { name: d.archived, exact: true }).click();
    await expect(card).toBeVisible();
    const rejected = page.waitForResponse((r) => r.url().endsWith("/preview"));
    await button(page, d.preview).click();
    expect((await (await rejected).json()).code).toBe("DASHBOARD_ARCHIVED");
    await button(card, english ? "More actions" : "更多操作").click();
    await reader
      .getByRole("menuitem", { name: d.restore, exact: true })
      .click();
    await change(reader);
    await reader
      .getByRole("button", { name: d.published, exact: true })
      .click();
    await expect(card).toContainText("First tab wins");
    await page.reload();
    await expect(button(page, d.metadata)).toBeVisible();
    const outdated = page.waitForResponse((r) => r.url().endsWith("/preview"));
    await button(page, d.preview).click();
    expect((await (await outdated).json()).code).toBe(
      "DASHBOARD_VERSION_CONFLICT",
    );
    const baseline = page.getByRole("dialog", {
      name: d.conflict,
      exact: true,
    });
    await expect(baseline).toContainText("Unpublished draft survives archive");
    await button(baseline, d.acknowledge).click();
    await expect(baseline).not.toBeVisible();
    await publish(page);
    await expect(
      page.getByText("Unpublished draft survives archive", { exact: false }),
    ).toBeVisible();
    await page.screenshot({
      path: testInfo.outputPath("three-signals-restored.png"),
      fullPage: true,
    });
    await reader.close();

    async function description(target: Page, value: string, status: number) {
      await button(target, d.metadata).click();
      await target.getByLabel(d.descriptionField).fill(value);
      const response = saveResponse(target);
      await button(target.getByRole("dialog"), d.done).click();
      expect((await response).status()).toBe(status);
    }
    async function publish(target: Page) {
      await button(target, d.preview).click();
      const review = target.getByRole("dialog", {
        name: d.previewTitle,
        exact: true,
      });
      const executed = target.waitForResponse(
        (r) => r.url().endsWith("/execute"),
        { timeout: 30000 },
      );
      await button(review, d.publish).click();
      await expect(target).toHaveURL(/\/dashboards\//);
      const response = await executed;
      expect(response.status()).toBe(200);
      const result = await response.json();
      expect(result.panels).toHaveLength(3);
      expect(
        result.panels.every((p: { status: string }) => p.status === "success"),
      ).toBe(true);
    }
    async function change(target: Page) {
      const review = target.getByRole("dialog", {
        name: d.changePreviewTitle,
        exact: true,
      });
      await button(review, english ? "Confirm" : "确认执行").click();
      await expect(review).not.toBeVisible();
    }
  });
}
const button = (scope: Page | Locator, name: string) =>
  scope.getByRole("button", { name, exact: true });
const saveResponse = (page: Page) =>
  page.waitForResponse(
    (r) =>
      r.request().method() === "PATCH" &&
      r.url().includes("/dashboard-drafts/"),
    { timeout: 30000 },
  );
async function select(
  page: Page,
  editor: Locator,
  name: string,
  value: string,
) {
  await selectTrigger(editor, name).click();
  await page.getByRole("option", { name: value, exact: true }).click();
}
