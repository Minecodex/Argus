import type { KubernetesWorkbenchDomains } from "../../kubernetes-workbench";
import type { HttpTransport } from "../../transport/http";

export function createKubernetesWorkbenchDomains(
  http: HttpTransport,
): KubernetesWorkbenchDomains {
  const base = (id: string) =>
    `enterprise/kubernetes-clusters/${encodeURIComponent(id)}/inventory`;
  return {
    kubernetesWorkbench: {
      helmReleases: (cluster, namespaceId, signal) =>
        http.request(
          `enterprise/kubernetes-clusters/${encodeURIComponent(cluster)}/helm/releases?${new URLSearchParams({ namespace_id: namespaceId })}`,
          { signal },
        ),
      helmHistory: (cluster, namespaceId, name, signal) =>
        http.request(
          `enterprise/kubernetes-clusters/${encodeURIComponent(cluster)}/helm/releases/${encodeURIComponent(name)}/history?${new URLSearchParams({ namespace_id: namespaceId })}`,
          { signal },
        ),
      previewHelm: (cluster, input) =>
        http.request(
          `enterprise/kubernetes-clusters/${encodeURIComponent(cluster)}/helm/actions/preview`,
          {
            method: "POST",
            body: input,
            csrf: true,
            headers: { "Idempotency-Key": crypto.randomUUID() },
          },
        ),
      createStreamSession: (cluster, input) =>
        http.request(
          `enterprise/kubernetes-clusters/${encodeURIComponent(cluster)}/stream-sessions`,
          { method: "POST", body: input, csrf: true },
        ),
      getStreamSession: (cluster, id, signal) =>
        http.request(
          `enterprise/kubernetes-clusters/${encodeURIComponent(cluster)}/stream-sessions/${encodeURIComponent(id)}`,
          { signal },
        ),
      closeStreamSession: (cluster, id) =>
        http.request(
          `enterprise/kubernetes-clusters/${encodeURIComponent(cluster)}/stream-sessions/${encodeURIComponent(id)}/close`,
          { method: "POST", csrf: true },
        ),
      issueStreamTicket: (cluster, id) =>
        http.request(
          `enterprise/kubernetes-clusters/${encodeURIComponent(cluster)}/stream-sessions/${encodeURIComponent(id)}/ticket`,
          { method: "POST", csrf: true },
        ),
      getLogSession: (clusterId, sessionId, signal) =>
        http.request(
          `enterprise/kubernetes-clusters/${encodeURIComponent(clusterId)}/log-sessions/${encodeURIComponent(sessionId)}`,
          { signal },
        ),
      createLogSession: (clusterId, input) =>
        http.request(
          `enterprise/kubernetes-clusters/${encodeURIComponent(clusterId)}/log-sessions`,
          { method: "POST", body: input, csrf: true },
        ),
      attachLogSession: (clusterId, sessionId, ticket) =>
        http.request(
          `enterprise/kubernetes-clusters/${encodeURIComponent(clusterId)}/log-sessions/${encodeURIComponent(sessionId)}/attach`,
          { method: "POST", body: { ticket }, csrf: true },
        ),
      readLogSession: (clusterId, sessionId, cursor, signal) =>
        http.request(
          `enterprise/kubernetes-clusters/${encodeURIComponent(clusterId)}/log-sessions/${encodeURIComponent(sessionId)}/read`,
          {
            method: "POST",
            body: cursor ? { cursor } : {},
            csrf: true,
            signal,
          },
        ),
      closeLogSession: (clusterId, sessionId) =>
        http.request(
          `enterprise/kubernetes-clusters/${encodeURIComponent(clusterId)}/log-sessions/${encodeURIComponent(sessionId)}/close`,
          { method: "POST", csrf: true },
        ),
      cancelDrain: (clusterId, id, expectedVersion) =>
        http.request(
          `enterprise/kubernetes-clusters/${encodeURIComponent(clusterId)}/operations/${encodeURIComponent(id)}`,
          {
            method: "POST",
            body: { expected_version: expectedVersion },
            csrf: true,
          },
        ),
      preview: (id, input) =>
        http.request(`${base(id)}/actions/preview-mutation`, {
          method: "POST",
          body: input,
          csrf: true,
          headers: { "Idempotency-Key": crypto.randomUUID() },
        }),
      operations: (id, input, signal) => {
        const query = new URLSearchParams();
        if (input?.limit) query.set("limit", String(input.limit));
        if (input?.cursor) query.set("cursor", input.cursor);
        return http.request(
          `enterprise/kubernetes-clusters/${encodeURIComponent(id)}/operations?${query}`,
          { signal },
        );
      },
      operation: (id, operationId, signal) =>
        http.request(
          `enterprise/kubernetes-clusters/${encodeURIComponent(id)}/operations/${encodeURIComponent(operationId)}`,
          { signal },
        ),
      watch: (id, token, signal) =>
        http.request(`${base(id)}/watch`, {
          method: "POST",
          body: { token },
          csrf: true,
          signal,
        }),
      scope: (id, signal) => http.request(`${base(id)}/scope`, { signal }),
      types: (id, signal) => http.request(`${base(id)}/types`, { signal }),
      list: (id, input, signal) => {
        const query = new URLSearchParams({
          type: input.type,
          namespace_mode:
            input.namespace_mode ??
            (input.namespace_ids !== undefined ? "explicit" : "all_authorized"),
        });
        for (const key of ["name", "labels", "limit", "cursor"] as const) {
          const value = input[key];
          if (value !== undefined && value !== "")
            query.set(key, String(value));
        }
        if (input.namespace_ids?.length)
          query.set("namespace_ids", input.namespace_ids.join(","));
        return http.request(`${base(id)}?${query}`, { signal });
      },
      object: (id, input, signal) => {
        const query = new URLSearchParams({
          type: input.type,
          name: input.name,
          uid: input.uid,
        });
        if (input.namespace_id) query.set("namespace_id", input.namespace_id);
        return http.request(`${base(id)}/object?${query}`, { signal });
      },
    },
  };
}
