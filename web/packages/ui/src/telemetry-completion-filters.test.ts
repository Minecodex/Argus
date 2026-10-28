import { it, expect } from "vitest";
import { completionFilters } from "./telemetry-completion-filters";
it("retains KQL predicates and escaped literals while discovering the next value", () => {
  expect(
    completionFilters(
      "kql",
      'service_name:"checkout" AND severity_number >= 17 AND ',
    ),
  ).toEqual([
    { field: "service_name", operator: ":", values: ["checkout"] },
    { field: "severity_number", operator: ">=", values: ["17"] },
  ]);
  expect(completionFilters("kql", 'body:"a\\"b" AND ')).toEqual([
    { field: "body", operator: ":", values: ['a"b'] },
  ]);
});
it("refuses unrepresentable contexts instead of dropping conditions", () => {
  expect(
    completionFilters("kql", 'service_name:"a" OR service_name:"b" AND '),
  ).toBeUndefined();
  expect(
    completionFilters("kql", "* | parse json | where x=1 AND "),
  ).toBeUndefined();
});
it("narrows GraphQL candidates using complete filter arguments", () => {
  expect(
    completionFilters(
      "skywalking_graphql",
      '{ queryTraces(serviceName:"api", pageSize:20, ',
    ),
  ).toEqual([{ field: "service_name", operator: "=", values: ["api"] }]);
  expect(
    completionFilters("skywalking_graphql", "{ queryTraces(durationMin:1000, "),
  ).toBeUndefined();
});
