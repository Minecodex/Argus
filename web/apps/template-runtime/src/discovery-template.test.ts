import { expect, it } from "vitest";
import fixtures from "./fixtures.generated.json";

it("keeps meta discovery and preparation compact while business observation has reviewed views", () => {
  const compact = [
    "tool.search",
    "tool.describe",
    "tool.invoke",
    "telemetry.dashboard.catalog",
    "telemetry.dashboard.catalog.resources",
    "telemetry.dashboard.context.candidates",
    "telemetry.dashboard.context.resolve",
    "telemetry.dashboard.draft.create",
    "telemetry.dashboard.draft.save",
    "telemetry.dashboard.draft.validate",
    "telemetry.dashboard.query.cancel",
    "telemetry.dashboard.query.resume",
    "pending_action.cancel",
  ];
  for (const tool of compact) {
    const binding = fixtures.bindings.find((row) => row.Tool === tool)!;
    expect(binding.Mode).toBe("trace");
    expect(binding.Hash).toBe("");
  }
  expect(
    fixtures.bindings.find((row) => row.Tool === "pending_action.get")?.Mode,
  ).toBe("action");
  for (const tool of [
    "host.list",
    "application.list",
    "monitoring.collection.list",
    "telemetry.promql.query",
    "observe.resources.list",
    "observe.capabilities.get",
    "observe.catalog.query",
  ]) {
    const binding = fixtures.bindings.find((row) => row.Tool === tool)!;
    expect(binding.Mode).toBe("template");
    expect(binding.Hash).toMatch(/^[a-f0-9]{64}$/);
    expect(
      fixtures.assets[binding.Hash as keyof typeof fixtures.assets],
    ).toBeTruthy();
  }
  expect(
    fixtures.bindings.find((row) => row.Tool === "workflow.plan.propose")?.Mode,
  ).toBe("interaction");
});
