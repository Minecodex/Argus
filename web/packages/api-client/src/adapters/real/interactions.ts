import type { ConversationInteractionState } from "../../interactions";
import type { RealDomainContext } from "./context";
export function installInteractionDomains({
  client,
  http,
  idempotencyKey,
}: RealDomainContext): void {
  const path = (conversation: string, id?: string) =>
    `conversations/${encodeURIComponent(conversation)}/interactions${id ? `/${encodeURIComponent(id)}` : ""}`;
  client.interactions = {
    list: (conversation) =>
      http.request<ConversationInteractionState>(path(conversation)),
    get: (conversation, id) => http.request(path(conversation, id)),
    respond: (conversation, id, body) =>
      http.request(`${path(conversation, id)}/respond`, {
        method: "POST",
        csrf: true,
        headers: { "Idempotency-Key": idempotencyKey() },
        body,
      }),
  };
}
