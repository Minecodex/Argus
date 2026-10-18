import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi, type InteractionResponse } from "@argus/api-client";
export const interactionQueryKey = (id?: string) =>
  ["conversation-interactions", id] as const;
export function useConversationInteractions(conversationId?: string) {
  const api = useApi(),
    cache = useQueryClient();
  const query = useQuery({
    queryKey: interactionQueryKey(conversationId),
    queryFn: () => api.interactions.list(conversationId!),
    enabled: !!conversationId,
    staleTime: 500,
    refetchInterval: (q) => (q.state.data?.active_run_id ? 1000 : false),
  });
  const respond = async (id: string, response: InteractionResponse) => {
    if (!conversationId) throw new Error("RUN_STATE_CONFLICT");
    await api.interactions.respond(conversationId, id, response);
    await Promise.all([
      cache.invalidateQueries({
        queryKey: interactionQueryKey(conversationId),
      }),
      cache.invalidateQueries({
        queryKey: ["conversations", conversationId, "events"],
      }),
      cache.invalidateQueries({
        queryKey: ["conversations", "detail", conversationId],
      }),
    ]);
  };
  return {
    ...query,
    pending: query.data?.items.find((item) => item.status === "pending"),
    respond,
  };
}
