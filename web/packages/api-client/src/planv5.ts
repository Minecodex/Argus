import type { components as MCP } from "./generated/mcpapi";
import type { components as Files } from "./generated/workspaceapi";
import type { components as Presentation } from "./generated/presentationapi";
import type { components as Conversation } from "./generated/conversationapi";
export type MCPConnection = MCP["schemas"]["MCPConnection"];
export type MCPConnectionWrite = MCP["schemas"]["MCPConnectionWrite"];
export type Workspace = Files["schemas"]["Workspace"];
export type WorkspaceFile = Files["schemas"]["WorkspaceFile"];
export type WorkspaceUpload = Files["schemas"]["WorkspaceUpload"];
export type ToolPresentation = Presentation["schemas"]["ToolPresentation"];
export type ConversationPreflight =
  Conversation["schemas"]["ConversationPreflight"];
export interface PlanV5Domains {
  mcp: {
    list(): Promise<MCPConnection[]>;
    create(input: MCPConnectionWrite): Promise<MCPConnection>;
    update(id: string, input: MCPConnectionWrite): Promise<MCPConnection>;
    test(id: string): Promise<MCPConnection>;
    setState(
      id: string,
      status: "enabled" | "disabled",
      expectedVersion: number,
    ): Promise<MCPConnection>;
    setMembers(
      id: string,
      memberIds: string[],
      expectedVersion: number,
    ): Promise<MCPConnection>;
  };
  workspace: {
    get(conversationId: string): Promise<Workspace>;
    remove(conversationId: string): Promise<Workspace>;
    listFiles(conversationId: string): Promise<WorkspaceFile[]>;
    upload(
      conversationId: string,
      file: File,
      onProgress: (percent: number) => void,
      signal?: AbortSignal,
    ): Promise<WorkspaceFile>;
    downloadUrl(
      conversationId: string,
      id: string,
      kind: "file" | "delivery",
    ): string;
  };
  presentations: {
    get(conversationId: string, toolCallId: string): Promise<ToolPresentation>;
    result(ref: string): Promise<import("./generated/contracts").ToolResult>;
  };
}
