import type { DashboardDomains } from "../../dashboard";
import type { HttpTransport } from "../../transport/http";

export function createDashboardDomains(
  http: HttpTransport,
  key: () => string,
): DashboardDomains {
  const draft = (id: string) => `dashboard-drafts/${encodeURIComponent(id)}`;
  const item = (id: string) => `dashboards/${encodeURIComponent(id)}`;
  const queries = (conversation: string, id?: string) =>
    `conversations/${encodeURIComponent(conversation)}/dashboard-queries${id ? `/${encodeURIComponent(id)}` : ""}`;
  const preview = <T>(path: string, body: unknown) =>
    http.request<T>(path, {
      method: "POST",
      body,
      csrf: true,
      headers: { "Idempotency-Key": key() },
    });
  return {
    dashboardQueries: {
      start: (conversation, body, requestKey) =>
        http.request(queries(conversation), {
          method: "POST",
          csrf: true,
          body,
          headers: { "Idempotency-Key": requestKey },
        }),
      get: (conversation, id) => http.request(queries(conversation, id)),
      cancel: (conversation, id) =>
        http.request(`${queries(conversation, id)}/cancel`, {
          method: "POST",
          csrf: true,
        }),
      resume: (conversation, id, expected_version) =>
        http.request(`${queries(conversation, id)}/resume`, {
          method: "POST",
          csrf: true,
          body: { expected_version },
        }),
    },
    dashboards: {
      convertPanel: (body) =>
        http.request("dashboard-spec/convert-panel", {
          method: "POST",
          csrf: true,
          body,
        }),
      bindings: (id) => http.request(`${item(id)}/bindings`),
      resourceBindings: (type, id) =>
        http.request(
          `${type === "host" ? "hosts" : "kubernetes/clusters"}/${encodeURIComponent(id)}/dashboard-bindings`,
        ),
      previewBinding: (type, id, body) =>
        preview(
          `${type === "host" ? "hosts" : "kubernetes/clusters"}/${encodeURIComponent(id)}/dashboard-bindings/preview`,
          body,
        ),
      list: () => http.request("dashboards"),
      get: (id) => http.request(item(id)),
      revisions: (id) => http.request(`${item(id)}/revisions`),
      drafts: () => http.request("dashboard-drafts"),
      draft: (id) => http.request(draft(id)),
      createDraft: (body) =>
        http.request("dashboard-drafts", { method: "POST", csrf: true, body }),
      saveDraft: (id, body) =>
        http.request(draft(id), { method: "PATCH", csrf: true, body }),
      discardDraft: (id, expected_version) =>
        http.request(draft(id), {
          method: "DELETE",
          csrf: true,
          body: { expected_version },
        }),
      rebase: (id, body) =>
        http.request(`${draft(id)}/rebase`, {
          method: "POST",
          csrf: true,
          body,
        }),
      validate: (body) =>
        http.request("dashboard-spec/validate", {
          method: "POST",
          csrf: true,
          body,
        }),
      sample: (id, expected_version, signal) =>
        http.request(`${draft(id)}/sample`, {
          method: "POST",
          body: { expected_version },
          signal,
        }),
      preview: (id, expected_version) =>
        preview(`${draft(id)}/preview`, { expected_version }),
      execute: (id, body, signal) =>
        http.request(`${item(id)}/execute`, { method: "POST", body, signal }),
      catalog: (body, signal) =>
        http.request("dashboards/catalog/query", {
          method: "POST",
          body,
          signal,
        }),
      drilldown: (id, body, signal) =>
        http.request(`${item(id)}/drilldown`, { method: "POST", body, signal }),
      generateDrilldowns: (id, body) =>
        http.request(`${draft(id)}/drilldowns/generate`, {
          method: "POST",
          csrf: true,
          body,
        }),
      folders: () => http.request("dashboard-folders"),
      previewFolder: (body) => preview("dashboard-folders/preview", body),
      previewLifecycle: (id, body) =>
        preview(`${item(id)}/actions/preview`, body),
    },
  };
}
