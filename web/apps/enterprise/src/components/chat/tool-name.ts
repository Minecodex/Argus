import type { TFunction } from "i18next";

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
