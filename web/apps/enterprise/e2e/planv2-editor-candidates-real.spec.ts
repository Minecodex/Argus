import { expect, test } from "@playwright/test";
import { createMfaLogin } from "./helpers/mfa-login";
import { dashboardAPI } from "./helpers/planv2-api";
import { dashboardsEn } from "../src/i18n/dashboards";

test("PlanV2 editor candidates real: saved signal, source, mappings and upstream values reach the catalog", async ({
  page,
}) => {
  test.skip(process.env.ARGUS_PLANV2_E2E !== "1", "Requires real telemetry");
  test.setTimeout(180000);
  await page.addInitScript(() => localStorage.setItem("argus.locale", "en-US"));
  await createMfaLogin("enterprise")(
    page,
    "/login",
    process.env.ARGUS_PLANV2_USERNAME!,
    process.env.ARGUS_PLANV2_PASSWORD!,
  );
  const gallery = await (
    await dashboardAPI(
      page,
      `/dashboards/${process.env.ARGUS_PLANV2_GALLERY_ID!}`,
    )
  ).json();
  const parameters = await (
    await dashboardAPI(
      page,
      `/dashboards/${process.env.ARGUS_PLANV2_PARAMETERS_ID!}`,
    )
  ).json();
  const spec = structuredClone(gallery.revision.spec);
  const panel = spec.panels.find(
    (p: { signal: string }) => p.signal === "metrics",
  );
  const upstream = structuredClone(
    parameters.revision.spec.variables.find(
      (v: { name: string }) => v.name === "pool",
    ),
  );
  expect(upstream).toBeTruthy();
  const value = "blue";
  upstream.default = { all: false, values: [value] };
  upstream.label = "Upstream";
  const query = {
    signal: "logs",
    source_binding: upstream.query.source_binding,
    field: "severity_text",
    filters: [
      { field: "severity_text", operator: "=", variable: upstream.name },
    ],
    parameter_bindings: [
      {
        parameter: upstream.name,
        variable: upstream.name,
        value_map: { blue: "INFO", green: "ERROR" },
      },
    ],
  };
  panel.local_filters = [
    {
      id: "candidate",
      label: "Local log body",
      kind: "query",
      multiple: false,
      required: false,
      default: { all: true, values: [] },
      query,
    },
  ];
  panel.drilldowns = [];
  spec.panels = [panel];
  spec.variables = [
    ...spec.variables.filter((v: { name: string }) => v.name !== upstream.name),
    upstream,
  ];
  const draft = await (
    await dashboardAPI(
      page,
      "/dashboard-drafts",
      "POST",
      {
        name: "Editor candidate closure",
        description: "",
        spec,
        proposed_bindings: [],
      },
      201,
    )
  ).json();
  await page.goto(`/dashboard-drafts/${draft.id}/panels/${panel.id}`);
  await page
    .locator(".argus-panel-editor__query-fields summary")
    .filter({ hasText: dashboardsEn.dashboards.localFilters })
    .first()
    .click();
  const lookup = () =>
    page.waitForResponse(
      (r) =>
        r.url().endsWith("/dashboards/catalog/query") &&
        r.request().postDataJSON()?.field === query.field,
    );
  const first = lookup();
  await page
    .getByRole("button", { name: "Local log body", exact: true })
    .click();
  const response = await first;
  expect(response.status()).toBe(200);
  expect(response.request().postDataJSON()).toMatchObject({
    signal: "logs",
    source_binding: query.source_binding,
    field: query.field,
    filters: [{ field: query.field, operator: "=", values: ["INFO"] }],
  });
  expect((await response.json()).values).toContain("INFO");
  await page
    .getByRole("dialog", { name: "Local log body", exact: true })
    .getByRole("button", { name: "Cancel", exact: true })
    .click();
  await page
    .locator(".argus-panel-editor__query-fields summary")
    .filter({ hasText: dashboardsEn.dashboards.variables })
    .first()
    .click();
  await page.getByRole("button", { name: "Upstream", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Upstream", exact: true });
  await dialog
    .getByRole("radio", { name: "green", exact: true })
    .press("Space");
  await dialog.getByRole("button", { name: "Apply", exact: true }).click();
  const changed = lookup();
  await page
    .getByRole("button", { name: "Local log body", exact: true })
    .click();
  const second = await changed;
  expect(second.request().postDataJSON().filters).toEqual([
    { field: query.field, operator: "=", values: ["ERROR"] },
  ]);
  expect((await second.json()).values).toContain("ERROR");
});
