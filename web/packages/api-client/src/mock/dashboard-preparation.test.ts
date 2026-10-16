import { expect, it } from "vitest";
import { emptyDashboardSpec, type DashboardPanel } from "../dashboard";
import { createMockApiClient } from "./index";

it("saves explicit preparation once and keeps the draft intact on invalid preparation", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const panel: DashboardPanel = {
    id: "cpu",
    title: "CPU",
    description: "",
    type: "line",
    signal: "metrics",
    authoring_mode: "builder",
    applicable_resource_types: ["host"],
    source_binding: { source_type: "node_exporter", capability_version: "v1" },
    local_filters: [],
    detail_query_targets: [],
    drilldowns: [],
    layout: { x: 0, y: 0, w: 6, h: 8, min_w: 2, min_h: 2 },
    unit: "percent",
    decimals: 2,
    legend: true,
    thresholds: [],
    targets: [
      {
        id: "q",
        language: "promql",
        query_mode: "range",
        parameter_bindings: [],
        range_step_policy: {
          kind: "auto",
          target_points: 300,
          min_step_seconds: 1,
        },
        source_definition: {
          builder: {
            operation: "value",
            metric: "cpu_utilization",
            filters: [],
            group_by: [],
          },
        },
      },
    ],
  };
  const spec = emptyDashboardSpec();
  spec.panels = [panel];
  const input = {
    name: "Prepared",
    description: "Keep this",
    spec,
    proposed_bindings: [],
  };
  const draft = await api.dashboards.createDraft(input);
  const saved = await api.dashboards.saveDraft(draft.id, {
    ...input,
    expected_version: draft.draft_version,
    preparation: {
      conversions: [{ panel_id: "cpu", mode: "dsl" }],
      drilldowns: [{ panel_id: "cpu", signal_sources: {} }],
    },
  });
  expect(saved.draft_version).toBe(draft.draft_version + 1);
  expect(saved.description).toBe(input.description);
  expect(saved.spec.panels[0]?.authoring_mode).toBe("dsl");
  expect(saved.spec.panels[0]?.drilldowns.length).toBeGreaterThan(0);
  expect(input.spec.panels[0]?.authoring_mode).toBe("builder");
  await expect(
    api.dashboards.saveDraft(draft.id, {
      ...input,
      expected_version: saved.draft_version,
      preparation: {
        conversions: [{ panel_id: "cpu", mode: "dsl" }],
        drilldowns: [{ panel_id: "missing", signal_sources: {} }],
      },
    }),
  ).rejects.toMatchObject({ code: "DASHBOARD_INVALID" });
  expect(await api.dashboards.draft(draft.id)).toEqual(saved);
  await expect(
    api.dashboards.saveDraft(draft.id, {
      ...input,
      expected_version: draft.draft_version,
    }),
  ).rejects.toMatchObject({ code: "DASHBOARD_VERSION_CONFLICT" });
});
