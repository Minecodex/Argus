import { expect, it } from "vitest";
import { emptyDashboardSpec, type DashboardPanel } from "../dashboard";
import { validateMockDashboard } from "./dashboard-state";
import { createMockApiClient } from "./index";

function panel(): DashboardPanel {
  return {
    id: "metric",
    title: "Metric",
    description: "",
    type: "timeseries",
    signal: "metrics",
    authoring_mode: "dsl",
    applicable_resource_types: ["host"],
    source_binding: { source_type: "otlp", capability_version: "v1" },
    local_filters: [],
    detail_query_targets: [],
    drilldowns: [],
    layout: { x: 0, y: 0, w: 6, h: 8, min_w: 2, min_h: 2 },
    unit: "",
    decimals: 2,
    legend: true,
    thresholds: [],
    targets: [
      {
        id: "a",
        language: "promql",
        query_mode: "range",
        parameter_bindings: [],
        range_step_policy: { kind: "auto", target_points: 300 },
        source_definition: { dsl: { expression: "up" } },
      },
    ],
  };
}
it("blocks reversed bounds, unordered thresholds and chart-incompatible settings", () => {
  const spec = emptyDashboardSpec();
  for (const patch of [
    { display: { min: 1, max: 0 } },
    {
      thresholds: [
        { value: 2, tone: "warning" },
        { value: 1, tone: "danger" },
      ],
    },
    { display: { reducer: "mean" } },
    { display: { draw_style: "bar", smooth: true } },
    { type: "stat", display: { stack: true } },
  ] as Partial<DashboardPanel>[]) {
    spec.panels = [{ ...panel(), ...patch }];
    expect(validateMockDashboard(spec).valid).toBe(false);
  }
});

it("preserves presentation through private drafts and publication without changing returned query data", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const spec = emptyDashboardSpec();
  spec.panels = [panel()];
  const draft = await api.dashboards.createDraft({
    name: "Display",
    description: "",
    proposed_bindings: [],
    spec,
  });
  const input = { from: "2026-09-25T00:00:00Z", to: "2026-09-25T01:00:00Z" };
  const first = await api.dashboards.preview(draft.id, draft.draft_version);
  await api.approvals.confirm(first.action_ref);
  const id = (await api.dashboards.list())[0]!.id;
  const before = await api.dashboards.execute(id, input);
  const edit = await api.dashboards.createDraft({
    dashboard_id: id,
    name: "Display",
    description: "",
    proposed_bindings: [],
    spec,
  });
  const display = {
    min: 0,
    max: 100,
    draw_style: "area" as const,
    stack: true,
    smooth: true,
  };
  spec.panels[0]!.display = display;
  spec.panels[0]!.thresholds = [{ value: 80, tone: "danger" }];
  const saved = await api.dashboards.saveDraft(edit.id, {
    name: "Display",
    description: "",
    proposed_bindings: [],
    spec,
    expected_version: edit.draft_version,
  });
  expect((await api.dashboards.draft(edit.id)).spec.panels[0]!.display).toEqual(
    display,
  );
  expect(
    (await api.dashboards.get(id)).revision.spec.panels[0]!.display,
  ).toBeUndefined();
  const preview = await api.dashboards.preview(edit.id, saved.draft_version);
  await api.approvals.confirm(preview.action_ref);
  const items = await api.dashboards.list();
  const published = await api.dashboards.get(items[0]!.id);
  expect(published.revision.spec.panels[0]!.display).toEqual(display);
  const after = await api.dashboards.execute(items[0]!.id, input);
  expect(after.panels[0]!.targets[0]!.data).toEqual(
    before.panels[0]!.targets[0]!.data,
  );
});
