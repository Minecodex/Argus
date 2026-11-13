import type { ConfirmActionResult } from "@argus/api-client";
export function confirmedKubernetesOperation(
  result: ConfirmActionResult,
): string | undefined {
  if (result.execution?.operation_ref?.kind === "kubernetes_operation")
    return result.execution.operation_ref.id;
  if (result.execution?.resource_ref?.resource_type === "kubernetes_operation")
    return result.execution?.resource_ref.resource_id;
  return undefined;
}
