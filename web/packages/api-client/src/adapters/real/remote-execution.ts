import type {
  RemoteExecution,
  RemoteExecutionDomains,
} from "../../remote-execution";
import type { RealDomainContext } from "./context";
export function installRemoteExecutionDomains({
  client,
  http,
  idempotencyKey,
}: RealDomainContext): void {
  const path = (conversation: string, id?: string) =>
    `conversations/${encodeURIComponent(conversation)}/remote-executions${id ? `/${encodeURIComponent(id)}` : ""}`;
  const domain: RemoteExecutionDomains["remoteExecutions"] = {
    inputUrl:(conversation,id,fileId)=>http.resolve(`${path(conversation,id)}/input-files/${encodeURIComponent(fileId)}`).href,
    async list(conversation) {
      return (
        await http.request<{ items: RemoteExecution[] }>(path(conversation))
      ).items;
    },
    get: (conversation, id) => http.request(path(conversation, id)),
    stop: (conversation, id) =>
      http.request(`${path(conversation, id)}/stop`, {
        method: "POST",
        csrf: true,
        headers: { "Idempotency-Key": idempotencyKey() },
      }),
    feedback: (conversation, id, feedback) =>
      http.request(`${path(conversation, id)}/feedback`, {
        method: "POST",
        csrf: true,
        headers: { "Idempotency-Key": idempotencyKey() },
        body: { feedback },
      }),
  };
  client.remoteExecutions = domain;
}
