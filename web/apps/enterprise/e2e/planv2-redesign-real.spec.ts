import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { createMfaLogin } from "./helpers/mfa-login";
import { dashboardAPI } from "./helpers/planv2-api";
import { observationRows } from "../../../packages/ui/src/observation-data";
import { dashboardsEn } from "../src/i18n/dashboards";
const login = createMfaLogin("enterprise"),
  enabled = process.env.ARGUS_PLANV2_E2E === "1";
test.describe("PlanV2 redesign real", () => {
  test.skip(!enabled, "PlanV2 Kubernetes environment is not active");
  test.setTimeout(180000);
  test("scopes an owned draft preview, preserves style data and publishes once", async ({
    page,
  }, info) => {
    await page.addInitScript(() => {
      localStorage.setItem("argus.locale", "en-US");
      localStorage.setItem("argus.theme", "dark");
    });
    await login(
      page,
      "/login",
      process.env.ARGUS_PLANV2_USERNAME ?? "",
      process.env.ARGUS_PLANV2_PASSWORD ?? "",
    );
    const id = process.env.ARGUS_PLANV2_GALLERY_ID!;
    await page.goto(`/dashboards/${id}`);
    const draftResponse = page.waitForResponse(
      (r) =>
        r.request().method() === "GET" &&
        /\/dashboard-drafts\/[^/]+$/.test(new URL(r.url()).pathname) &&
        r.status() === 200,
    );
    await page.getByRole("button", { name: "Edit", exact: true }).click();
    const draft = await (await draftResponse).json();
    const panel = draft.spec.panels.find(
      (p: { signal: string }) => p.signal === "metrics",
    );
    expect(panel).toBeTruthy();
    await page
      .getByRole("button", { name: `Edit panel ${panel.title}`, exact: true })
      .click();
    await expect(page).toHaveURL(/\/panels\//);
    const response = page.waitForResponse(
      (r) =>
        r.url().endsWith(`/dashboard-drafts/${draft.id}/sample`) &&
        r.status() === 200,
    );
    await page.getByRole("button", { name: "Run query", exact: true }).click();
    const sampled = await (await response).json();
    expect(sampled.validation.valid).toBe(true);
    expect(
      sampled.sample.execution.panels.map((p: { id: string }) => p.id),
    ).toEqual([panel.id]);
    expect(sampled.sample.execution.context_token).toBeTruthy();
    expect(sampled.sample.execution.from).toBeTruthy();
    expect(sampled.sample.execution.panels[0].sources.length).toBeGreaterThan(
      0,
    );
    const queries: string[] = [];
    page.on("request", (r) => {
      if (r.url().endsWith("/sample")) queries.push(r.url());
    });
    await page.getByRole("tab", { name: "Style", exact: true }).click();
    await page.getByLabel("Decimal places").fill("3");
    await expect(page.getByText("Draft saved", { exact: false })).toBeVisible();
    expect(queries).toHaveLength(0);
    await expect(
      page.getByText("Latest query result", { exact: true }),
    ).toBeVisible();
    const result = await new AxeBuilder({ page }).analyze();
    expect(
      result.violations.filter((v) =>
        ["serious", "critical"].includes(v.impact),
      ),
    ).toEqual([]);
    await page.screenshot({ path: info.outputPath("real-editor.png") });
    await page.reload();
    await page.getByRole("tab", { name: "Style", exact: true }).click();
    await expect(page.getByLabel("Decimal places")).toHaveValue("3");
    await page.getByRole("button", { name: "Done", exact: true }).click();
    await page.getByRole("button", { name: "Preview and publish" }).click();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Confirm publication", exact: true })
      .click();
    await expect(page).toHaveURL(/\/dashboards\//);
  });
  test("draft navigation freezes real data and rejects changed queries and published-context reuse", async ({
    page,
  }) => {
    await page.addInitScript(() =>
      localStorage.setItem("argus.locale", "en-US"),
    );
    await login(
      page,
      "/login",
      process.env.ARGUS_PLANV2_USERNAME!,
      process.env.ARGUS_PLANV2_PASSWORD!,
    );
    const board = process.env.ARGUS_PLANV2_DASHBOARD_ID!;
    const published = await (
      await dashboardAPI(page, `/dashboards/${board}`)
    ).json();
    const draft = await (
      await dashboardAPI(
        page,
        "/dashboard-drafts",
        "POST",
        {
          dashboard_id: board,
          name: published.revision.name,
          description: published.revision.description,
          spec: published.revision.spec,
          proposed_bindings: [],
        },
        201,
      )
    ).json();
    const panel = draft.spec.panels.find(
      (p: { type: string }) => p.type === "apm_services",
    );
    expect(panel).toBeTruthy();
    const sampled = await (
      await dashboardAPI(page, `/dashboard-drafts/${draft.id}/sample`, "POST", {
        expected_version: draft.draft_version,
        parameters: {
          panel_ids: [panel.id],
          resource_ids: [process.env.ARGUS_PLANV2_CLUSTER_ID!],
        },
      })
    ).json();
    expect(sampled.validation.valid).toBe(true);
    const execution = sampled.sample.execution;
    expect(execution.panels.map((p: { id: string }) => p.id)).toEqual([
      panel.id,
    ]);
    const drill = panel.drilldowns.find(
      (d: { origin_query_ref: string; scope_policy: string }) =>
        d.origin_query_ref === panel.targets[0].id &&
        d.scope_policy === "inherit",
    );
    expect(drill).toBeTruthy();
    const rows = observationRows(execution.panels[0].targets[0].data);
    expect(rows.length).toBeGreaterThan(0);
    const values = Object.fromEntries(
      Object.entries(drill.inputs).map(([key, path]) => [
        key,
        String(
          (path as string)
            .slice(1)
            .split("/")
            .reduce<unknown>(
              (value, part) =>
                value && typeof value === "object"
                  ? (value as Record<string, unknown>)[
                      part.replaceAll("~1", "/").replaceAll("~0", "~")
                    ]
                  : undefined,
              rows[0],
            ),
        ),
      ]),
    );
    const input = {
      context_token: execution.context_token,
      panel_id: panel.id,
      drilldown_id: drill.id,
      values,
      expand_authorized_resources: false,
    };
    const initial = await (
      await dashboardAPI(
        page,
        `/dashboard-drafts/${draft.id}/drilldown`,
        "POST",
        { ...input, expected_version: draft.draft_version },
      )
    ).json();
    expect(initial.execution.from).toBe(execution.from);
    expect(initial.execution.to).toBe(execution.to);
    expect(initial.execution.result.status).toBe("success");
    const spec = structuredClone(draft.spec);
    spec.panels.find((p: { id: string }) => p.id === panel.id).decimals = 4;
    const save = async (version: number) =>
      (
        await dashboardAPI(page, `/dashboard-drafts/${draft.id}`, "PATCH", {
          name: draft.name,
          description: draft.description,
          folder_id: draft.folder_id,
          spec,
          proposed_bindings: draft.proposed_bindings,
          expected_version: version,
        })
      ).json();
    const styled = await save(draft.draft_version);
    const reused = await (
      await dashboardAPI(
        page,
        `/dashboard-drafts/${draft.id}/drilldown`,
        "POST",
        { ...input, expected_version: styled.draft_version },
      )
    ).json();
    expect(reused.draft_version).toBe(styled.draft_version);
    await dashboardAPI(
      page,
      `/dashboards/${board}/drilldown`,
      "POST",
      input,
      403,
    );
    spec.panels
      .find((p: { id: string }) => p.id === panel.id)
      .targets[0].source_definition.builder.filters.push({
        field: "environment",
        operator: "=",
        value: "changed-query",
      });
    const changed = await save(styled.draft_version);
    const stale = await (
      await dashboardAPI(
        page,
        `/dashboard-drafts/${draft.id}/drilldown`,
        "POST",
        { ...input, expected_version: changed.draft_version },
        409,
      )
    ).json();
    expect(stale.code).toBe("DASHBOARD_VERSION_CONFLICT");
  });
  test("changing preview scope cancels the old request and ignores its late response", async ({
    page,
  }) => {
    await page.addInitScript(() =>
      localStorage.setItem("argus.locale", "en-US"),
    );
    await login(
      page,
      "/login",
      process.env.ARGUS_PLANV2_USERNAME!,
      process.env.ARGUS_PLANV2_PASSWORD!,
    );
    await page.goto(`/dashboards/${process.env.ARGUS_PLANV2_GALLERY_ID!}`);
    const fetched = page.waitForResponse(
      (r) =>
        r.request().method() === "GET" &&
        /\/dashboard-drafts\/[^/]+$/.test(new URL(r.url()).pathname),
    );
    await page.getByRole("button", { name: "Edit", exact: true }).click();
    const draft = await (await fetched).json(),
      panel = draft.spec.panels.find(
        (p: { signal: string }) => p.signal === "metrics",
      );
    await page
      .getByRole("button", { name: `Edit panel ${panel.title}`, exact: true })
      .click();
    const seedResponse = page.waitForResponse(
      (r) =>
        r.url().endsWith(`/dashboard-drafts/${draft.id}/sample`) &&
        r.status() === 200,
    );
    await page.getByRole("button", { name: "Run query", exact: true }).click();
    const original = await (await seedResponse).json();
    let release: () => void = () => {},
      captured: (value: unknown) => void = () => {},
      attempts = 0;
    const gate = new Promise<void>((resolve) => (release = resolve)),
      first = new Promise<unknown>((resolve) => (captured = resolve));
    await page.route(
      `**/dashboard-drafts/${draft.id}/sample`,
      async (route) => {
        if (++attempts !== 1) {
          await route.continue();
          return;
        }
        // Browser fetch uses the suite's TLS host mapping. Reuse its real
        // response to simulate delayed delivery without a second Node dial.
        captured(original);
        await gate;
        await route
          .fulfill({
            status: 200,
            contentType: "application/json",
            json: original,
          })
          .catch(() => {});
      },
    );
    try {
      await page
        .getByRole("button", { name: "Run query", exact: true })
        .click();
      await first;
      await page
        .getByRole("button", {
          name: dashboardsEn.dashboards.time,
          exact: true,
        })
        .click();
      const range = page.getByRole("dialog", {
        name: dashboardsEn.dashboards.time,
        exact: true,
      });
      await range.getByLabel(dashboardsEn.dashboards.timeSeconds).fill("7200");
      await range.getByRole("button", { name: "Apply", exact: true }).click();
      const response = page.waitForResponse(
        (r) =>
          r.url().endsWith(`/dashboard-drafts/${draft.id}/sample`) &&
          r.status() === 200,
      );
      await page
        .getByRole("button", { name: "Run query", exact: true })
        .click();
      const current = await (await response).json();
      expect(current.sample.execution.execution_hash).not.toBe(
        original.sample.execution.execution_hash,
      );
      await expect(
        page.getByText("Latest query result", { exact: true }),
      ).toBeVisible();
      release();
      await expect(
        page.getByRole("button", { name: "Run query", exact: true }),
      ).toBeVisible();
      await expect(
        page.getByText("Latest query result", { exact: true }),
      ).toBeVisible();
    } finally {
      release();
    }
  });
});
