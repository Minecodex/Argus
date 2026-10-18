import type { components } from "./generated/remoteexecutionapi";
export type RemoteExecution = components["schemas"]["RemoteExecution"];
export type ConversationMode = "execute" | "plan";
export interface RemoteExecutionDomains {
  remoteExecutions: {
    inputUrl(
      conversationId: string,
      id: string,
      fileId: string,
    ): string | undefined;
    list(conversationId: string): Promise<RemoteExecution[]>;
    get(conversationId: string, id: string): Promise<RemoteExecution>;
    stop(conversationId: string, id: string): Promise<RemoteExecution>;
    feedback(
      conversationId: string,
      id: string,
      feedback: string,
    ): Promise<RemoteExecution>;
  };
}
