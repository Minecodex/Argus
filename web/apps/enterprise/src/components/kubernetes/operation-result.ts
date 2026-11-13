import type {
  KubernetesOperation,
  KubernetesResourceReference,
} from "@argus/api-client";
import { asRecord } from "./resource-patch";
export function operationResource(
  operation: KubernetesOperation,
): KubernetesResourceReference | undefined {
  if (
    operation.status !== "succeeded" ||
    operation.result.deleted ||
    operation.result.delete_requested ||
    operation.operation.startsWith("helm_")
  )
    return;
  const after = asRecord(operation.result.after),
    meta = asRecord(after.metadata);
  const uid =
    typeof operation.result.uid === "string"
      ? operation.result.uid
      : operation.uid;
  const name = typeof meta.name === "string" ? meta.name : operation.name;
  if (!uid || !name) return;
  return {
    operation_id: operation.id,
    type: operation.type,
    name,
    uid,
    namespace: typeof meta.namespace === "string" ? meta.namespace : "",
    namespace_uid: operation.namespace_uid,
    namespace_id: operation.namespace_id,
  };
}
