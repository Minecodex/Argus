import type {
  AgentEvent,
  ConversationEvent,
} from "@argus/api-client/contracts";
import type { ChatFile, ChatMessage } from "./chat-view-model";

export function fileFromPublic(value: unknown): ChatFile | null {
  if (!value || typeof value !== "object") return null;
  const data = value as Record<string, unknown>;
  if (typeof data.id !== "string" || typeof data.name !== "string") return null;
  const string = (key: string) =>
    typeof data[key] === "string" ? (data[key] as string) : undefined;
  return {
    id: data.id,
    name: data.name,
    byte_size: Number(data.byte_size) || 0,
    path: string("path"),
    media_type: string("media_type"),
    content_hash: string("content_hash"),
  };
}

/** Files are service activities regardless of actor or the presence of a Run. */
export function fileEntryFromEvent(
  event: ConversationEvent | AgentEvent,
): ChatMessage | null {
  if (
    event.event_type !== "workspace_file_added" &&
    event.event_type !== "artifact_published"
  )
    return null;
  const payload = event.payload as Record<string, unknown>;
  if (payload.metadata_index === true) return null;
  const delivery = event.event_type === "artifact_published";
  const file = fileFromPublic(delivery ? payload.artifact : payload.file);
  if (!file) return null;
  return {
    id: event.event_id,
    conversationId: event.conversation_id ?? "",
    createdAt: event.occurred_at,
    sequence: event.sequence,
    runId: event.run_id,
    kind: "file",
    role: "system",
    content: "",
    fileGroup: delivery
      ? undefined
      : file.path?.match(/^\/workspace\/queries\/([^/]+)\//)?.[1],
    ...(delivery ? { artifacts: [file] } : { files: [file] }),
  };
}

export function appendFileEntry(
  entries: ChatMessage[],
  entry: ChatMessage,
): void {
  const previous = entries[entries.length - 1];
  if (
    entry.fileGroup &&
    previous?.kind === "file" &&
    previous.fileGroup === entry.fileGroup &&
    previous.files?.length
  ) {
    entries[entries.length - 1] = {
      ...previous,
      files: [
        ...previous.files,
        ...(entry.files ?? []).filter(
          (file) => !previous.files?.some((stored) => stored.id === file.id),
        ),
      ],
    };
  } else entries.push(entry);
}
