import type { AlertRule, AlertPolicySpec } from "@argus/api-client";

// The numeric builder owns an observation and separate thresholds. Advanced
// mode changes only the editing surface, preserving the exact PromQL bytes.
export function canUseRuleBuilder(rule: AlertRule): boolean {
  return (
    rule.query_mode === "numeric" &&
    /^[a-zA-Z_:][a-zA-Z0-9_:]*(?:\{[^{}]*\})?$/.test(rule.expression.trim())
  );
}
export function ruleEditingMode(rule: AlertRule): "builder" | "advanced" {
  return canUseRuleBuilder(rule) ? "builder" : "advanced";
}

// Builder-created rules have no separate name control. Derive only absent
// names from their own expressions before trial or preview schema validation.
export function policyWithRuleNames(policy: AlertPolicySpec): AlertPolicySpec {
  const copy = structuredClone(policy);
  copy.rules = copy.rules.map((rule) => ({
    ...rule,
    name: rule.name.trim() || rule.expression.trim(),
  }));
  return copy;
}
