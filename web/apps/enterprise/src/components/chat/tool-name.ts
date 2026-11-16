import type { TFunction } from "i18next";
import type { ToolCallTrace } from "./chat-view-model";

export function toolTraceName(t: TFunction, call: ToolCallTrace) {
  const name = toolDisplayName(t, call.toolName, call.source);
  return call.requestedTarget &&
    ["tool.invoke", "tool.describe"].includes(call.toolName) &&
    (!call.source || call.source === "argus")
    ? `${name} · ${call.requestedTarget}`
    : name;
}

/** Translate display text while preserving the exact routing/audit identity. */
export function toolDisplayName(
  t: TFunction,
  toolName: string,
  source?: string,
): string {
  if (source && source !== "argus" && source !== "sandbox") return toolName;
  return t(`toolNames.labels.${toolName.replaceAll(".", "/")}`, {
    defaultValue: toolName,
  });
}
