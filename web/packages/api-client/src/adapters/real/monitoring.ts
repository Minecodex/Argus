import type { MonitoringDomains, MonitoringListInput } from "../../monitoring";
import type { HttpTransport } from "../../transport/http";
import { monitoringRequest } from "../../monitoring-request";

export function createMonitoringDomains(
  http: HttpTransport,
  key: () => string,
): MonitoringDomains {
  const query = (path: string, input: MonitoringListInput = {}) => {
    const params = new URLSearchParams();
    for (const field of [
      "search",
      "environment",
      "cursor",
      "limit",
      "status",
      "integration",
      "desired_enabled",
      "apply_status",
      "health",
      "freshness",
    ] as const) {
      const value = input[field];
      if (value !== undefined && value !== "") params.set(field, String(value));
    }
    if (input.application_ids !== undefined) {
      params.set("application_mode", "explicit");
      if (input.application_ids.length)
        params.set("application_ids", input.application_ids.join(","));
    }
    return `${path}?${params}`;
  };
  return {
    monitoring: {
      ingestConnection: (id) =>
        http.request(
          `monitoring/ingest-endpoints/${encodeURIComponent(id)}/connection`,
        ),
      builtins: (target) =>
        http.request(
          `monitoring/builtins?target_type=${encodeURIComponent(target.kind)}&target_id=${encodeURIComponent(target.id)}`,
        ),
      previewBuiltin: (input) =>
        http.request("monitoring/builtins/preview", {
          method: "POST",
          csrf: true,
          headers: { "Idempotency-Key": key() },
          body: input,
        }),
      ingestEndpoints: (id, input) =>
        http.request(
          query(
            `applications/${encodeURIComponent(id)}/ingest-endpoints`,
            input,
          ),
        ),
      ingestKeys: (id, input) =>
        http.request(
          query(
            `monitoring/ingest-endpoints/${encodeURIComponent(id)}/keys`,
            input,
          ),
        ),
      revealIngestKey: (id) =>
        http.request(
          `monitoring/ingest-keys/${encodeURIComponent(id)}/reveal`,
          { method: "POST", csrf: true },
        ),
      collectors: (input) =>
        http.request(query("monitoring/collectors", input)),
      integrations: () => http.request("monitoring/integrations"),
      collections: (input) =>
        http.request(query("monitoring/collections", input)),
      collection: (id) =>
        http.request(`monitoring/collections/${encodeURIComponent(id)}`),
      targets: (input) => http.request(query("monitoring/targets", input)),
      applications: (input) => http.request(query("applications", input)),
      application: (id) =>
        http.request(`applications/${encodeURIComponent(id)}`),
      services: (input) => http.request(query("applications/services", input)),
      preview: (input) =>
        http.request("monitoring/commands/preview", {
          method: "POST",
          body: monitoringRequest(input),
          csrf: true,
          headers: { "Idempotency-Key": key() },
        }),
    },
  };
}
