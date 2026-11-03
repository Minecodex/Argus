import type { ArgusApiClient, ObservationResourceRef } from "@argus/api-client";

export const observationReferenceTypes = [
  "host",
  "kubernetes_cluster",
  "kubernetes_namespace",
  "application",
  "monitoring_target",
] as const;

export function isObservationReferenceType(
  type: string,
): type is ObservationResourceRef["type"] {
  return observationReferenceTypes.some((kind) => kind === type);
}

/** A management read does not establish access to an observation reference. */
export async function readObservationReference(
  api: Pick<ArgusApiClient, "telemetry">,
  ref: ObservationResourceRef,
) {
  const page = await api.telemetry.resources({
    kind: ref.type,
    search: ref.id,
    limit: 1,
  });
  const item = page.items.find(
    (row) => row.id === ref.id && row.type === ref.type,
  );
  if (!item) throw new Error("Resource unavailable");
  return item;
}
