import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  formatApiError,
  type ConversationMode,
  type ConversationAccessMode,
} from "@argus/api-client";
import { Button, ComposerActions } from "@argus/ui";
import { ShieldCheck, ShieldAlert, Hand } from "lucide-react";
export function useConversationModes(
  conversationId: string | undefined,
  prepareConversation: () => Promise<string>,
) {
  const api = useApi(),
    { t } = useTranslation(),
    cache = useQueryClient();
  const [saving, setSaving] = useState(false),
    [error, setError] = useState<string | null>(null);
  const query = useQuery({
    queryKey: ["conversations", "detail", conversationId],
    queryFn: () => api.conversations.get(conversationId!),
    enabled: !!conversationId,
  });
  const change = async (
    kind: "mode" | "access",
    value: ConversationMode | ConversationAccessMode,
  ) => {
    setSaving(true);
    setError(null);
    try {
      const id = conversationId ?? (await prepareConversation());
      await api.conversations.get(id);
      const updated =
        kind === "mode"
          ? await api.conversations.updateMode(id, value as ConversationMode)
          : await api.conversations.updateAccessMode(
              id,
              value as ConversationAccessMode,
            );
      cache.setQueryData(["conversations", "detail", id], updated);
      await cache.invalidateQueries({ queryKey: ["conversations"] });
    } catch (e) {
      setError(
        formatApiError(e, t("plan7.failure"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      setSaving(false);
    }
  };
  return {
    mode: query.data?.mode ?? "execute",
    access: query.data?.access_mode ?? "request_approval",
    saving,
    error,
    changeMode: (value: ConversationMode) => change("mode", value),
    changeAccess: (value: ConversationAccessMode) => change("access", value),
  };
}
export function ChatAccessSwitch({
  value,
  disabled,
  onChange,
}: {
  value: ConversationAccessMode;
  disabled?: boolean;
  onChange(value: ConversationAccessMode): void;
}) {
  const { t } = useTranslation();
  return (
    <ComposerActions
      label={t("chatInteractions.permission")}
      trigger={
        <Button
          variant="ghost"
          size="sm"
          isDisabled={disabled}
          className={
            value === "full_access"
              ? "argus-chat-access argus-chat-access--full"
              : "argus-chat-access"
          }
        >
          {value === "full_access" ? (
            <ShieldAlert size={15} />
          ) : (
            <ShieldCheck size={15} />
          )}
          {t(
            value === "full_access"
              ? "chatInteractions.fullAccess"
              : "chatInteractions.requestApproval",
          )}
        </Button>
      }
      sections={[
        {
          label: t("chatInteractions.permission"),
          items: [
            {
              id: "request_approval",
              label: t("chatInteractions.requestApproval"),
              description: t("chatInteractions.requestDescription"),
              icon: <Hand size={16} />,
              selected: value === "request_approval",
              onSelect: () => onChange("request_approval"),
            },
            {
              id: "full_access",
              label: t("chatInteractions.fullAccess"),
              description: t("chatInteractions.fullDescription"),
              icon: <ShieldAlert size={16} />,
              selected: value === "full_access",
              onSelect: () => onChange("full_access"),
            },
          ],
        },
      ]}
    />
  );
}
