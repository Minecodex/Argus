import type { AlertDomains } from "../../alerts";
import type { HttpTransport } from "../../transport/http";
export function createAlertDomains(
  http: HttpTransport,
  key: () => string,
): AlertDomains {
  const post = <T>(path: string, body: unknown) =>
    http.request<T>(`alerts/${path}`, {
      method: "POST",
      body,
      csrf: true,
      headers: { "Idempotency-Key": key() },
    });
  const query = (path: string, filter: object) => {
    const params = new URLSearchParams();
    for (const [k, v] of Object.entries(filter)) {
      if (v !== undefined && v !== null && v !== "") params.set(k, String(v));
    }
    return `${path}?${params}`;
  };
  const item = (collection: string, id: string) =>
    `alerts/${collection}/${encodeURIComponent(id)}`;
  return {
    alerts: {
      summary: () => http.request("alerts/summary"),
      configurations: (kind) =>
        http.request(query("alerts/configurations", { kind })),
      configuration: (id) => http.request(item("configurations", id)),
      preview: (input) => post("configurations/preview", input),
      previewCommand: (input) => post("commands/preview", input),
      trial: (input) => post("trial", input),
      instances: (filter = {}, history = false) =>
        http.request(
          query(history ? "alerts/history" : "alerts/instances", filter),
        ),
      instance: (id) => http.request(item("instances", id)),
      handle: (id, input) =>
        post(`instances/${encodeURIComponent(id)}/handle`, input),
      deliveries: (filter = {}) =>
        http.request(query("alerts/deliveries", filter)),
      delivery: (id) => http.request(item("deliveries", id)),
      exportHistory: (filter = {}) =>
        http.request(query("alerts/history/export", filter)),
    },
  };
}
