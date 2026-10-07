import { readFileSync } from "node:fs";
import { expect, it, vi } from "vitest";
import { HttpTransport } from "../../transport/http";
import { emptyDashboardSpec, type DashboardPanel } from "../../dashboard";
import { createDashboardDomains } from "./dashboard";
type Node = {
  $ref?: string;
  parameters?: Node[];
  in?: string;
  name?: string;
  required?: boolean;
  [key: string]: unknown;
};
it("dashboard adapter uses declared HTTP operations and required confirmation headers", async () => {
  const contract = JSON.parse(
    readFileSync(
      new URL(
        "../../../../../../api/openapi/generated/argus.bundle.json",
        import.meta.url,
      ),
      "utf8",
    ),
  ) as { paths: Record<string, Node> };
  const resolve = (node: Node): Node => {
    if (!node.$ref) return node;
    let value: unknown = contract;
    for (const key of node.$ref.slice(2).split("/"))
      value = (value as Record<string, unknown>)[
        key.replaceAll("~1", "/").replaceAll("~0", "~")
      ];
    return resolve(value as Node);
  };
  const fetch = vi.fn(
    async (input: string | URL | Request, init?: RequestInit) => {
      const path = new URL(String(input)).pathname.replace(/^\/api\/v1/, "");
      const match = Object.entries(contract.paths).find(([pattern]) =>
        new RegExp(`^${pattern.replace(/\{[^}]+\}/g, "[^/]+")}$`).test(path),
      );
      expect(match, path).toBeDefined();
      const method = (init?.method ?? "GET").toLowerCase();
      const operation = match![1][method] as Node;
      expect(operation, `${method} ${path}`).toBeDefined();
      for (const parameter of [
        ...(match![1].parameters ?? []),
        ...(operation.parameters ?? []),
      ].map(resolve))
        if (parameter.in === "header" && parameter.required)
          expect(new Headers(init?.headers).has(parameter.name!)).toBe(true);
      return new Response("{}", {
        headers: { "Content-Type": "application/json" },
      });
    },
  );
  const domains = createDashboardDomains(
    new HttpTransport({
      base_url: "https://argus.example.test",
      fetch,
      csrf_token: () => "test-csrf",
    }),
    () => "test-key",
  );
  const api = domains.dashboards;
  const id = "11111111-1111-4111-8111-111111111111",
    spec = emptyDashboardSpec(),
    draft = { name: "dashboard", description: "", spec, proposed_bindings: [] };
  await api.list();
  await api.get(id);
  await api.revisions(id);
  await api.drafts();
  await api.draft(id);
  await api.createDraft(draft);
  await api.saveDraft(id, { ...draft, expected_version: 1 });
  await api.discardDraft(id, 1);
  await api.rebase(id, {
    revision_id: id,
    expected_version: 1,
    object_version: 1,
  });
  await api.validate(spec);
  const panel: DashboardPanel = {
    id: "p",
    title: "CPU",
    description: "",
    type: "timeseries",
    signal: "metrics",
    authoring_mode: "builder",
    source_binding: { source_type: "otlp", capability_version: "v1" },
    applicable_resource_types: ["host"],
    local_filters: [],
    targets: [
      {
        id: "q",
        language: "promql",
        query_mode: "range",
        range_step_policy: {
          kind: "auto",
          target_points: 100,
          min_step_seconds: 1,
        },
        source_definition: {
          builder: {
            operation: "value",
            metric: "cpu",
            filters: [],
            group_by: [],
          },
        },
        parameter_bindings: [],
      },
    ],
    detail_query_targets: [],
    drilldowns: [],
    layout: { x: 0, y: 0, w: 6, h: 20, min_w: 3, min_h: 10 },
    unit: "",
    decimals: 2,
    legend: true,
    thresholds: [],
  };
  await api.convertPanel({ panel, mode: "dsl" });
  await api.preview(id, 1);
  await api.folders();
  await api.bindings(id);
  for (const type of ["host", "kubernetes_cluster"] as const) {
    await api.resourceBindings(type, id);
    await api.previewBinding(type, id, {
      operation: "attach",
      dashboard_id: id,
      expected_dashboard_version: 1,
      expected_resource_version: 1,
    });
    await api.previewBinding(type, id, {
      operation: "detach",
      dashboard_id: id,
      expected_dashboard_version: 1,
      expected_resource_version: 1,
      binding_id: id,
      expected_binding_version: 1,
    });
  }
  const lifecycle = {
    operation: "folder.create" as const,
    name: "folder",
    description: "",
    expected_version: 0,
    sort_order: 0,
  };
  await api.previewFolder(lifecycle);
  await api.previewLifecycle(id, {
    ...lifecycle,
    operation: "archive",
    expected_version: 1,
  });
  await api.generateDrilldowns(id, {
    panel_id: "a",
    expected_version: 1,
    signal_sources: {},
  });
  const controller = new AbortController();
  await api.execute(id, {}, controller.signal);
  await api.sample(id, { expected_version: 1 }, controller.signal);
  await api.catalog(
    {
      kind: "metrics",
      signal: "metrics",
      source_binding: { source_type: "otlp", capability_version: "v1" },
      resource_ids: [],
      selected_values: [],
      filters: [],
      from: new Date(0).toISOString(),
      to: new Date().toISOString(),
      limit: 20,
    },
    controller.signal,
  );
  await api.drilldown(
    id,
    {
      context_token: "context",
      panel_id: "a",
      drilldown_id: "open",
      values: {},
      expand_authorized_resources: false,
    },
    controller.signal,
  );
  expect(
    fetch.mock.calls
      .slice(-4)
      .every(([, options]) => options?.signal === controller.signal),
  ).toBe(true);
  await domains.dashboardQueries.start(
    id,
    {
      dashboard_id: id,
      parameters: {
        resource_ids: [],
        panel_ids: [],
        variables: {},
        local_values: {},
      },
    },
    "stable-query-key",
  );
  await domains.dashboardQueries.get(id, id);
  await domains.dashboardQueries.cancel(id, id);
  await domains.dashboardQueries.resume(id, id, 3);
  expect(
    new Headers(fetch.mock.calls.at(-4)?.[1]?.headers).get("Idempotency-Key"),
  ).toBe("stable-query-key");
  await domains.dashboardQueries.start(
    id,
    {
      dashboard_id: id,
      parameters: {},
      drilldown: {
        parent_job_id: id,
        panel_id: "traces",
        drilldown_id: "detail",
        values: { trace_id: "known" },
        expand_authorized_resources: false,
      },
    },
    "detail-key",
  );
  expect(JSON.parse(fetch.mock.calls.at(-1)![1]!.body as string)).toMatchObject(
    {
      drilldown: {
        parent_job_id: id,
        drilldown_id: "detail",
        expand_authorized_resources: false,
      },
    },
  );
});
