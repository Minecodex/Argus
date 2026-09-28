import { expect, it } from "vitest";
import { createMockApiClient } from "./index";
import { emptyDashboardSpec, type DashboardPanel } from "../dashboard";

it("freezes query definitions, delivers marked mock files, and isolates cancelled and foreign conversations", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const conversation = await api.conversations.create();
  const panel: DashboardPanel = {
    id: "logs",
    title: "Logs",
    description: "",
    type: "logs",
    signal: "logs",
    authoring_mode: "builder",
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
        language: "kql",
        query_mode: "",
        parameter_bindings: [],
        range_step_policy: {
          kind: "auto",
          target_points: 300,
          min_step_seconds: 1,
        },
        source_definition: {
          builder: { operation: "records", filters: [], group_by: [] },
        },
      },
    ],
  };
  const spec = emptyDashboardSpec();
  spec.panels = [panel];
  const draft = await api.dashboards.createDraft({
    name: "Query",
    description: "",
    spec,
    proposed_bindings: [],
  });
  const generated = await api.dashboards.generateDrilldowns(draft.id, {
    expected_version: draft.draft_version,
    panel_id: "logs",
    signal_sources: {},
  });
  const preview = await api.dashboards.preview(
    draft.id,
    generated.draft.draft_version,
  );
  await api.approvals.confirm(preview.action_ref);
  const item = (await api.dashboards.list())[0]!;
  const input = {
    dashboard_id: item.id,
    parameters: {
      resource_ids: [],
      panel_ids: [],
      variables: {},
      local_values: {},
    },
  };
  const job = await api.dashboardQueries.start(
    conversation.id,
    input,
    "query-test",
  );
  await expect(
    api.dashboardQueries.start(
      conversation.id,
      { ...input, parameters: { ...input.parameters, candidates_only: true } },
      "candidates-only",
    ),
  ).rejects.toMatchObject({ code: "DASHBOARD_INVALID" });
  expect(job.status).toBe("queued");
  expect(
    (await api.dashboardQueries.start(conversation.id, input, "query-test")).id,
  ).toBe(job.id);
  await expect(
    api.dashboardQueries.start(
      conversation.id,
      { ...input, parameters: { ...input.parameters, panel_ids: ["logs"] } },
      "query-test",
    ),
  ).rejects.toMatchObject({ code: "DASHBOARD_VERSION_CONFLICT" });
  const other = await api.conversations.create();
  await expect(
    api.dashboardQueries.get(other.id, job.id),
  ).rejects.toMatchObject({ code: "DASHBOARD_NOT_FOUND" });
  const result = await api.dashboardQueries.get(conversation.id, job.id);
  expect(result.status).toBe("complete");
  expect(result.revision_id).toBe(item.active_revision_id);
  expect(result.manifest?.analysis_status).toBe("not_analyzed");
  expect(result.manifest?.warnings).toEqual(["MOCK_DATA"]);
  expect(result.files).toHaveLength(2);
  const files = await api.workspace.listFiles(conversation.id);
  expect(files).toHaveLength(2);
  const body = result.files.find((f) => f.kind === "data")!;
  const url = api.workspace.downloadUrl(
    conversation.id,
    body.workspace_file_id!,
    "file",
  );
  expect(JSON.parse(atob(url.split(",")[1]!))[0].service_name).toBe("checkout");
  await api.dashboardQueries.get(conversation.id, job.id);
  expect(await api.workspace.listFiles(conversation.id)).toHaveLength(2);
  const inspect = generated.draft.spec.panels[0]!.drilldowns[0]!;
  const detailInput = {
    dashboard_id: item.id,
    parameters: {},
    drilldown: {
      parent_job_id: job.id,
      panel_id: "logs",
      drilldown_id: inspect.id,
      values: {},
      expand_authorized_resources: false,
    },
  };
  const detail = await api.dashboardQueries.start(
    conversation.id,
    detailInput,
    "detail",
  );
  const expanded = await api.dashboardQueries.get(conversation.id, detail.id);
  expect(expanded.status).toBe("complete");
  expect(expanded.revision_id).toBe(result.revision_id);
  expect(expanded.manifest?.drilldown).toMatchObject({
    parent_job_id: job.id,
    depth: 1,
    target_id: inspect.detail_query_ref,
  });
  expect(await api.workspace.listFiles(conversation.id)).toHaveLength(4);
  await expect(
    api.dashboardQueries.start(
      conversation.id,
      {
        ...detailInput,
        parameters: { variables: { env: { all: true, values: [] } } },
      },
      "override",
    ),
  ).rejects.toMatchObject({ code: "DASHBOARD_INVALID" });
  await expect(
    api.dashboardQueries.start(
      conversation.id,
      {
        ...detailInput,
        drilldown: {
          ...detailInput.drilldown,
          expand_authorized_resources: true,
        },
      },
      "expand",
    ),
  ).rejects.toMatchObject({ code: "DASHBOARD_INVALID" });
  const cancelled = await api.dashboardQueries.start(
    conversation.id,
    input,
    "cancel-test",
  );
  await api.dashboardQueries.cancel(conversation.id, cancelled.id);
  expect(
    (await api.dashboardQueries.get(conversation.id, cancelled.id)).status,
  ).toBe("cancelled");
  expect(await api.workspace.listFiles(conversation.id)).toHaveLength(4);
});
