import { useTranslation } from "react-i18next";
import { useConversationResources } from "./conversation-resources";
import type { ChatMessage } from "./chat-view-model";
import { Badge } from "@argus/ui";

export function ConversationResourceIndex({
  messages,
}: {
  messages: ChatMessage[];
}) {
  const { t } = useTranslation();
  const resources = useConversationResources(messages);
  if (!resources.length) return null;
  return (
    <details open>
      <summary>
        {t("chat.resourceDetails.references")} · {resources.length}
      </summary>
      <ul className="argus-chat-resource-list">
        {resources.map((ref) => (
          <li key={`${ref.type}:${ref.id}`}>
            <Badge>{t(`chat.resourceTypes.${ref.type}`)}</Badge>
            {ref.unavailable ? (
              <span>{t("chat.composer.resourceFailed")}</span>
            ) : ref.href ? (
              <a href={ref.href}>
                {ref.name ?? t("chat.composer.resourceLoading")}
              </a>
            ) : (
              <span>{ref.name ?? t("chat.composer.resourceLoading")}</span>
            )}
          </li>
        ))}
      </ul>
    </details>
  );
}
