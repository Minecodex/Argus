export interface MockToolCallTrace {
  callId: string;
  toolName: string;
  status: "running" | "success" | "failed";
  summary?: string;
  durationMs?: number;
  startedAt: string;
}

export interface MockChatMessage {
  dashboardContext?: import("../generated/contracts").DashboardChatContext;
  id: string;
  conversationId: string;
  role: "user" | "assistant" | "system";
  content: string;
  createdAt: string;
  modelId?: string;
  modelRevision?: number;
  inputPricePerMillionSnapshot?: number;
  outputPricePerMillionSnapshot?: number;
  inputTokens?: number;
  outputTokens?: number;
  toolCalls?: MockToolCallTrace[];
  presentations?: string[];
  pendingActionRefs?: string[];
}

export type MockChatStreamEvent =
  | { type: "message_start"; messageId: string }
  | { type: "token"; messageId: string; delta: string }
  | { type: "tool_call"; messageId: string; toolCall: MockToolCallTrace }
  | {
      type: "tool_call_update";
      messageId: string;
      callId: string;
      status: "success" | "failed";
      durationMs: number;
      summary?: string;
    }
  | { type: "presentation"; messageId: string; toolCallId: string }
  | { type: "pending_action"; messageId: string; actionRef: string }
  | { type: "message_done"; message: MockChatMessage }
  | { type: "error"; message: string };
