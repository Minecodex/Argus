import { describe, expect, it } from "vitest";
import { canUseRuleBuilder, ruleEditingMode } from "./rule-query";
import type { AlertRule } from "@argus/api-client";
const rule = (expression: string): AlertRule => ({
  expression,
  name: "CPU",
  query_mode: "numeric",
  operator: ">",
  threshold: 85,
  recovery_threshold: 75,
  severity: "critical",
  unit: "%",
});
describe("rule editor conversion", () => {
  it("retains a numeric observation without folding its trigger into the query", () => {
    const input = rule('host_cpu_usage_percent{env="production"}');
    const original = structuredClone(input);
    expect(canUseRuleBuilder(input)).toBe(true);
    expect(ruleEditingMode(input)).toBe("builder");
    expect(input).toEqual(original);
  });
  it("keeps advanced aggregation and predicates in their original editing source", () => {
    expect(
      ruleEditingMode(
        rule("sum by (argus_resource_id) (rate(cpu_seconds_total[5m]))"),
      ),
    ).toBe("advanced");
    expect(
      canUseRuleBuilder({
        ...rule("cpu > 85"),
        query_mode: "predicate",
        presence_expression: "cpu",
      }),
    ).toBe(false);
  });
});
