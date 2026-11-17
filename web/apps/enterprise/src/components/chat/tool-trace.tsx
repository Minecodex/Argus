import type { ToolCallTrace } from "./chat-view-model";
import { ChatToolActivity } from "./chat-tool-activity";
export function ToolTrace({
  toolCalls,
  label,
}: {
  toolCalls: ToolCallTrace[];
  label?: string;
}) {
  return toolCalls.length ? (
    <div data-testid="tool-trace">
      {toolCalls.map((call) => (
        <ChatToolActivity key={call.callId} call={call} label={label} />
      ))}
    </div>
  ) : null;
}
