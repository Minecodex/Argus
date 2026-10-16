import { expect, it } from "vitest";
import fixtures from "./fixtures.generated.json";

it("keeps discovery and preparation in compact calls without allocating an iframe asset", () => {
  const compact = ["tool.search", "tool.describe", "tool.invoke", "telemetry.dashboard.catalog", "telemetry.dashboard.catalog.resources", "telemetry.dashboard.context.candidates", "telemetry.dashboard.context.resolve", "telemetry.dashboard.draft.create", "telemetry.dashboard.draft.save", "telemetry.dashboard.draft.validate", "telemetry.dashboard.query.cancel", "telemetry.dashboard.query.resume", "pending_action.cancel"];
  for (const tool of compact) {
    const binding = fixtures.bindings.find(row => row.Tool === tool)!;
    expect(binding.Mode).toBe("trace");
    expect(binding.Hash).toBe("");
  }
  expect(fixtures.bindings.find(row => row.Tool === "pending_action.get")?.Mode).toBe("action");
  expect(fixtures.bindings.filter(row => row.Mode === "template")).toHaveLength(90);
});
