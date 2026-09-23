import type {
  PlanV5Domains,
  MCPConnection,
  Workspace,
  WorkspaceFile,
  WorkspaceUpload,
  ToolPresentation,
} from "../../planv5";
import type { HttpTransport } from "../../transport/http";

export function createPlanV5Domains(
  http: HttpTransport,
  key: () => string,
): PlanV5Domains {
  const mcp = "enterprise/mcp-connections";
  const workspace = (id: string) =>
    `conversations/${encodeURIComponent(id)}/workspace`;
  return {
    mcp: {
      async list() {
        return (await http.request<{ items: MCPConnection[] }>(mcp)).items;
      },
      create: (body) =>
        http.request(mcp, {
          method: "POST",
          csrf: true,
          headers: { "Idempotency-Key": key() },
          body,
        }),
      update: (id, body) =>
        http.request(`${mcp}/${id}`, { method: "PUT", csrf: true, body }),
      test: (id) =>
        http.request(`${mcp}/${id}/test`, {
          method: "POST",
          csrf: true,
          headers: { "Idempotency-Key": key() },
        }),
      setState: (id, status, version) =>
        http.request(`${mcp}/${id}/state`, {
          method: "PUT",
          csrf: true,
          body: { status, expected_version: version },
        }),
      setMembers: (id, memberIds, version) =>
        http.request(`${mcp}/${id}/members`, {
          method: "PUT",
          csrf: true,
          body: { member_ids: memberIds, expected_version: version },
        }),
    },
    workspace: {
      get: (id) => http.request<Workspace>(workspace(id)),
      remove: (id) =>
        http.request(workspace(id), {
          method: "DELETE",
          csrf: true,
          headers: { "Idempotency-Key": key() },
        }),
      async listFiles(id) {
        return (
          await http.request<{ items: WorkspaceFile[] }>(
            `${workspace(id)}/files`,
          )
        ).items;
      },
      async upload(id, file, progress, signal) {
        const upload = await http.request<WorkspaceUpload>(
          `${workspace(id)}/uploads`,
          {
            method: "POST",
            csrf: true,
            headers: { "Idempotency-Key": key() },
            body: { name: file.name, byte_size: file.size },
            signal,
          },
        );
        return http.upload<WorkspaceFile>(
          `${workspace(id)}/uploads/${upload.id}/content`,
          file,
          progress,
          signal,
        );
      },
      downloadUrl: (id, file, kind) =>
        http
          .resolve(
            kind === "file"
              ? `${workspace(id)}/files/${encodeURIComponent(file)}/content`
              : `conversations/${encodeURIComponent(id)}/deliveries/${encodeURIComponent(file)}/content`,
          )
          .toString(),
    },
    presentations: {
      result: (ref) => http.request(`tool-results/${encodeURIComponent(ref)}`),
      get: (id, call) =>
        http.request<ToolPresentation>(
          `conversations/${encodeURIComponent(id)}/tool-presentations/${encodeURIComponent(call)}`,
        ),
    },
  };
}
