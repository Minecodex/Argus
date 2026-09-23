import type {
  MCPConnection,
  PlanV5Domains,
  ToolPresentation,
  Workspace,
  WorkspaceFile,
} from "../planv5";
import type { BaseContext, MockContext } from "./context";
import { ApiError } from "../transport/errors";

export type MockPlanV5 = {
  connections: { enterpriseId: string; value: MCPConnection }[];
  selections: Record<string, string[]>;
  workspaces: Record<string, Workspace>;
  files: Record<string, WorkspaceFile[]>;
  contents: Record<string, string>;
  presentations: Record<
    string,
    { conversationId: string; value: ToolPresentation }
  >;
};
const copy = <T>(value: T): T => structuredClone(value);
async function hash(value: ArrayBuffer): Promise<string> {
  const result = await crypto.subtle.digest("SHA-256", value);
  return Array.from(new Uint8Array(result), (x) =>
    x.toString(16).padStart(2, "0"),
  ).join("");
}
export async function publishMockPresentation(
  ctx: BaseContext,
  conversationId: string,
  toolCallId: string,
  data: Record<string, unknown>,
) {
  const source =
    '<section id="result"></section><script>ArgusTemplate.onData((data)=>{const root=document.getElementById("result");root.replaceChildren();ArgusTemplate.table(root,data.items||[],["name","status","hostname"]);});</script>';
  ctx.db.planv5.presentations[toolCallId] = {
    conversationId,
    value: {
      tool_call_id: toolCallId,
      runtime: "argus-template/v1",
      status: "ready",
      template_source: source,
      template_hash: await hash(new TextEncoder().encode(source).buffer),
      detail_data: data,
    },
  };
  ctx.save();
}
export function createPlanV5Domains(ctx: MockContext): PlanV5Domains {
  const state = ctx.db.planv5;
  const conversation = (id: string) =>
    ctx.mustFind(
      ctx.db.conversations,
      (item) =>
        item.id === id &&
        item.enterpriseId === ctx.enterpriseId() &&
        item.createdBy === ctx.actor().id,
      "conversation",
    );
  const connection = (id: string) =>
    ctx.mustFind(
      state.connections,
      (item) =>
        item.enterpriseId === ctx.enterpriseId() && item.value.id === id,
      "MCP connection",
    ).value;
  const checkVersion = (value: MCPConnection, version: number | undefined) => {
    if (value.version !== version) throw new Error("MCP_CONNECTION_CONFLICT");
  };
  const saveConnection = (value: MCPConnection) => {
    value.version++;
    value.updated_at = ctx.nowIso();
    ctx.save();
    return copy(value);
  };
  return {
    mcp: {
      async list() {
        return copy(
          state.connections
            .filter((item) => item.enterpriseId === ctx.enterpriseId())
            .map((item) => item.value),
        );
      },
      async create(input) {
        new URL(input.endpoint);
        const value: MCPConnection = {
          id: crypto.randomUUID(),
          name: input.name,
          endpoint: input.endpoint,
          auth_type: input.auth_type,
          member_ids: [...input.member_ids],
          transport: "streamable_http",
          status: "enabled",
          health_status: "unknown",
          version: 1,
          revision: 1,
          tool_count: 0,
          created_at: ctx.nowIso(),
          updated_at: ctx.nowIso(),
        };
        state.connections.push({ enterpriseId: ctx.enterpriseId(), value });
        ctx.save();
        return copy(value);
      },
      async update(id, input) {
        const value = connection(id);
        checkVersion(value, input.expected_version);
        value.name = input.name;
        value.endpoint = input.endpoint;
        value.auth_type = input.auth_type;
        value.member_ids = [...input.member_ids];
        value.revision++;
        value.health_status = "unknown";
        return saveConnection(value);
      },
      async test(id) {
        const value = connection(id);
        value.health_status = "healthy";
        value.tool_count = 2;
        return saveConnection(value);
      },
      async setState(id, status, version) {
        const value = connection(id);
        checkVersion(value, version);
        value.status = status;
        return saveConnection(value);
      },
      async setMembers(id, members, version) {
        const value = connection(id);
        checkVersion(value, version);
        value.member_ids = [...members];
        return saveConnection(value);
      },
    },
    workspace: {
      async get(id) {
        conversation(id);
        const value = state.workspaces[id];
        if (!value || value.status === "deleted")
          throw new ApiError(
            {
              code: "WORKSPACE_NOT_FOUND",
              message_key: "planv5.files.empty",
              request_id: "mock-workspace",
              retryable: false,
            },
            404,
          );
        return copy(value);
      },
      async remove(id) {
        conversation(id);
        const value = state.workspaces[id];
        if (!value) throw new Error("WORKSPACE_NOT_FOUND");
        value.status = "deleted";
        value.version++;
        for (const file of state.files[id] ?? [])
          delete state.contents[file.id];
        state.files[id] = [];
        ctx.save();
        return copy(value);
      },
      async listFiles(id) {
        conversation(id);
        return copy(state.files[id] ?? []);
      },
      async upload(id, file, progress, signal) {
        conversation(id);
        if (file.size > 100 * 1024 * 1024)
          throw new Error("WORKSPACE_FILE_TOO_LARGE");
        if (signal?.aborted)
          throw new DOMException("Upload cancelled", "AbortError");
        progress(0);
        const bytes = await file.arrayBuffer();
        const checksum = await hash(bytes);
        if (signal?.aborted)
          throw new DOMException("Upload cancelled", "AbortError");
        const encoded = await new Promise<string>((resolve, reject) => {
          const reader = new FileReader();
          reader.onerror = () => reject(reader.error);
          reader.onload = () => resolve(String(reader.result));
          reader.readAsDataURL(file);
        });
        const workspace = state.workspaces[id];
        if (!workspace || workspace.status === "deleted")
          state.workspaces[id] = {
            id: crypto.randomUUID(),
            conversation_id: id,
            status: "ready",
            capacity_bytes: 2 * 1024 ** 3,
            version: 1,
            created_at: ctx.nowIso(),
          };
        const fileId = crypto.randomUUID();
        const value: WorkspaceFile = {
          id: fileId,
          name: file.name,
          path: `/workspace/uploads/${fileId}/${file.name}`,
          byte_size: file.size,
          content_hash: checksum,
          media_type: file.type || "application/octet-stream",
          created_at: ctx.nowIso(),
        };
        (state.files[id] ??= []).push(value);
        state.contents[fileId] = encoded;
        ctx.save();
        progress(100);
        return copy(value);
      },
      downloadUrl(id, file) {
        conversation(id);
        if (!(state.files[id] ?? []).some((item) => item.id === file))
          throw new Error("WORKSPACE_FILE_NOT_FOUND");
        return state.contents[file]!;
      },
    },
    presentations: {
      async result(ref) {
        const call = ref.replace(/^result_/, "");
        const stored = state.presentations[call];
        if (!stored) throw new Error("TOOL_RESULT_NOT_FOUND");
        conversation(stored.conversationId);
        const data = JSON.stringify(stored.value.detail_data);
        const size = new TextEncoder().encode(data).length;
        const checksum = await hash(new TextEncoder().encode(data).buffer);
        return {
          result_ref: ref,
          tool_call_id: call,
          tool_id: "tool.invoke",
          byte_size: size,
          content_hash: checksum,
          partial: false,
          projection: {
            schema_version: "argus.tool_result_projection/v1",
            tool_call_id: call,
            projection_schema_version: "v1",
            result_ref: ref,
            result_hash: checksum,
            summary: stored.value.detail_data,
            resource_refs: [],
            partial: false,
            original_bytes: size,
            projected_bytes: size,
          },
        };
      },
      async get(id, call) {
        conversation(id);
        const value = state.presentations[call];
        if (!value || value.conversationId !== id)
          throw new Error("PRESENTATION_NOT_FOUND");
        return copy(value.value);
      },
    },
  };
}
