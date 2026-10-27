import type { MonitoringMutation } from "./monitoring";

// Read models include attested namespace/owner metadata. Only selected object
// identity belongs in a request; the server resolves current ownership again.
export function monitoringRequest(
  input: MonitoringMutation,
): MonitoringMutation {
  const config = input.collection?.config;
  const workload = config?.workload;
  if (!workload) return input;
  return {
    ...input,
    collection: {
      ...input.collection,
      config: {
        ...config,
        workload: {
          namespace_id: workload.namespace_id,
          type: workload.type,
          name: workload.name,
          uid: workload.uid,
          containers: [...workload.containers],
        },
      },
    },
  };
}
