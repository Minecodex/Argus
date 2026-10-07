import { expect, it } from "vitest";
import { candidateContext, candidateRequest } from "./candidate-filters";
import type { DashboardSchemas } from "@argus/api-client";
const query: DashboardSchemas["DashboardCandidateQuery"] = {
  signal: "logs",
  source_binding: { source_type: "filelog", capability_version: "v1" },
  metric: "different_metric",
  field: "service_name",
  filters: [
    { field: "env", operator: "=", variable: "env" },
    { field: "region", operator: "=", local_parameter: "region" },
  ],
  parameter_bindings: [
    { parameter: "env", variable: "env", value_map: { production: "prod" } },
  ],
};
const scope = {
  from: "2026-10-07T00:00:00Z",
  to: "2026-10-07T01:00:00Z",
  resource_ids: ["a"],
  variables: { env: { all: false, values: ["production"] } },
  locals: { region: { all: false, values: ["east"] } },
};
it("uses the entire saved candidate definition and mapped effective dependencies", () => {
  const input = candidateRequest(
    query,
    scope,
    { all: false, values: ["saved"] },
    "order",
    "page2",
  );
  expect(input).toMatchObject({
    signal: "logs",
    source_binding: query.source_binding,
    metric: "different_metric",
    field: "service_name",
    resource_ids: ["a"],
    selected_values: ["saved"],
    search: "order",
    cursor: "page2",
    filters: [
      { field: "env", operator: "=", values: ["prod"] },
      { field: "region", operator: "=", values: ["east"] },
    ],
  });
});
it("invalidates only referenced variables and locals, as well as time/resources/definitions", () => {
  const base = { ...scope, time: [scope.from, scope.to] };
  const key = candidateContext(query, base);
  expect(
    candidateContext(query, {
      ...base,
      variables: {
        ...base.variables,
        unrelated: { all: false, values: ["changed"] },
      },
    }),
  ).toBe(key);
  for (const changed of [
    { ...base, time: "changed" },
    { ...base, resource_ids: ["b"] },
    { ...base, variables: { env: { all: true, values: [] } } },
    { ...base, locals: { region: { all: false, values: ["west"] } } },
  ])
    expect(candidateContext(query, changed)).not.toBe(key);
  expect(
    candidateContext(
      {
        ...query,
        source_binding: { source_type: "otlp", capability_version: "v1" },
      },
      base,
    ),
  ).not.toBe(key);
});
it("refuses unresolved or unmapped dependencies and omits All filters", () => {
  expect(() =>
    candidateRequest(
      query,
      { ...scope, variables: {} },
      { all: true, values: [] },
      "",
    ),
  ).toThrow("UNRESOLVED");
  expect(() =>
    candidateRequest(
      query,
      { ...scope, variables: { env: { all: false, values: ["unknown"] } } },
      { all: true, values: [] },
      "",
    ),
  ).toThrow("UNMAPPED");
  expect(
    candidateRequest(
      query,
      { ...scope, variables: { env: { all: true, values: [] } } },
      { all: true, values: [] },
      "",
    ).filters,
  ).toHaveLength(1);
});
