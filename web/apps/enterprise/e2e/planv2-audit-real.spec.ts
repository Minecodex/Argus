import { expect, test, type Page } from "@playwright/test";
import type { DashboardSchemas } from "@argus/api-client";
import { settingsEn, settingsZh } from "../src/i18n/settings";
import { createMfaLogin } from "./helpers/mfa-login";
import { dashboardAPI } from "./helpers/planv2-api";

const login = createMfaLogin("enterprise");
test.use({ actionTimeout: 15000 });
for (const english of [false, true]) {
  test(`real audit filters, paging and immutable facts ${english ? "en dark" : "zh light"}`, async ({
    page,
  }, info) => {
    test.skip(
      process.env.ARGUS_PLANV2_E2E !== "1",
      "Requires PlanV2 Kubernetes environment",
    );
    test.setTimeout(240000);
    const a = (english ? settingsEn : settingsZh).settings.audit;
    await page.addInitScript(
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
    const gallery = process.env.ARGUS_PLANV2_GALLERY_ID!;
    const published = await (
      await dashboardAPI(page, `/dashboards/${gallery}`)
    ).json();
    const spec = { ...published.revision.spec, panels: [], variables: [] };
    const name = `PlanV2 audit ${english ? "en" : "zh"}`;
    const create = async (name: string) =>
      (await (
        await dashboardAPI(
          page,
          "/dashboard-drafts",
          "POST",
          { name, description: "", spec, proposed_bindings: [] },
          201,
        )
      ).json()) as DashboardSchemas["DashboardDraft"];
    let target = await create(name);
    for (let i = 0; i < 51; i++)
      target = await save(page, target, `revision-${i}`);
    let noise = await create(`${name} unrelated`);
    for (let i = 0; i < 51; i++) noise = await save(page, noise, `noise-${i}`);
    // Exact resource ID restricts both the event set and its signed cursor.
    const params = new URLSearchParams({
      action: "dashboard.draft.saved",
      resource_id: target.id,
      limit: "1",
    });
    const one = await (
      await dashboardAPI(page, `/enterprise/audit-events?${params}`)
    ).json();
    expect(one.items).toHaveLength(1);
    expect(one.items[0].details.draft_version).toBe(target.draft_version);
    expect(one.items[0].event_hash).toMatch(/^[a-f0-9]{64}$/);
    params.set("cursor", one.page.next_cursor);
    params.set("resource_id", noise.id);
    await dashboardAPI(
      page,
      `/enterprise/audit-events?${params}`,
      "GET",
      undefined,
      400,
    );

    await page.goto("/settings/audit");
    await choose(page, a.filters.actionType, a.actions.dashboard_draft_saved);
    await choose(page, a.filters.resourceType, a.resourceTypes.dashboard_draft);
    // Search the event's immutable draft ID, which is beyond the first page of unfiltered events.
    await page.getByPlaceholder(a.filters.searchPlaceholder).fill(target.id);
    const rows = page.getByRole("row").filter({ hasText: name });
    await expect(rows).toHaveCount(50);
    await expect(
      page.getByRole("button", { name: a.next, exact: true }),
    ).toBeEnabled();
    const firstID = await rows.first().getByRole("button").textContent();
    await rows.first().getByRole("button").click();
    const detail = page.getByRole("dialog", {
      name: a.detailTitle,
      exact: true,
    });
    await expect(detail).toContainText(
      `"draft_version": ${target.draft_version}`,
    );
    await expect(detail).toContainText("spec_hash");
    await expect(detail).toContainText(a.detail.eventHash);
    const factsView = detail.getByRole("region", {
      name: a.detail.facts,
      exact: true,
    });
    await factsView.scrollIntoViewIfNeeded();
    await expect(factsView).toBeVisible();
    await factsView.screenshot({
      path: info.outputPath("draft-audit-facts.png"),
    });
    await page.keyboard.press("Escape");
    await page.getByRole("button", { name: a.next, exact: true }).click();
    await expect(rows).toHaveCount(1);
    expect(await rows.first().getByRole("button").textContent()).not.toBe(
      firstID,
    );
    await expect(
      page.getByRole("button", { name: a.next, exact: true }),
    ).toBeDisabled();

    // Publication identifies the actual immutable revision, not only the draft.
    await choose(page, a.filters.actionType, a.actions.dashboard_published);
    await choose(page, a.filters.resourceType, a.resourceTypes.dashboard);
    await page.getByPlaceholder(a.filters.searchPlaceholder).fill(gallery);
    const publication = page
      .getByRole("row")
      .filter({ hasText: published.dashboard.name });
    await expect(publication).toHaveCount(1);
    await publication.getByRole("button").click();
    await expect(detail).toContainText(published.revision.id);
    await expect(detail).toContainText('"revision_number": 1');
    await expect(detail).not.toContainText("last_over_time");
    await factsView.scrollIntoViewIfNeeded();
    await factsView.screenshot({
      path: info.outputPath("publication-audit-facts.png"),
    });
    await page.keyboard.press("Escape");

    const execution = await (
      await dashboardAPI(page, `/dashboards/${gallery}/execute`, "POST", {
        resource_ids: [process.env.ARGUS_PLANV2_CLUSTER_ID!],
      })
    ).json();
    const events = await (
      await dashboardAPI(
        page,
        `/enterprise/audit-events?${new URLSearchParams({ action: "dashboard.query.executed", query: execution.execution_id })}`,
      )
    ).json();
    expect(events.items).toHaveLength(1);
    const facts = events.items[0].details;
    expect(facts.revision_id).toBe(published.revision.id);
    expect(facts.panels).toHaveLength(11);
    expect(
      facts.panels.every((p: { status: string }) => p.status === "success"),
    ).toBe(true);
    expect(facts.parameters_hash).toMatch(/^[a-f0-9]{64}$/);
    expect(JSON.stringify(facts)).not.toContain(execution.context_token);
    expect(JSON.stringify(facts)).not.toContain("last_over_time");
    await choose(
      page,
      a.filters.actionType,
      a.actions.dashboard_query_executed,
    );
    await page
      .getByPlaceholder(a.filters.searchPlaceholder)
      .fill(execution.execution_id);
    const executed = page
      .getByRole("row")
      .filter({ hasText: a.actions.dashboard_query_executed });
    await expect(executed).toHaveCount(1);
    await executed.getByRole("button").click();
    await expect(detail).toContainText(execution.execution_id);
    await factsView.scrollIntoViewIfNeeded();
    await factsView.screenshot({
      path: info.outputPath("execution-audit-facts.png"),
    });
  });
}
async function choose(page: Page, label: string, value: string) {
  // HeroUI includes the selected value before the field label in the trigger's name.
  const escaped = label.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  await page
    .locator(".argus-filter-bar")
    .getByRole("button", { name: new RegExp(`${escaped}$`) })
    .click();
  await page.getByRole("option", { name: value, exact: true }).click();
}
async function save(
  page: Page,
  draft: DashboardSchemas["DashboardDraft"],
  description: string,
) {
  return (await (
    await dashboardAPI(page, `/dashboard-drafts/${draft.id}`, "PATCH", {
      name: draft.name,
      description,
      spec: draft.spec,
      proposed_bindings: [],
      expected_version: draft.draft_version,
    })
  ).json()) as DashboardSchemas["DashboardDraft"];
}
