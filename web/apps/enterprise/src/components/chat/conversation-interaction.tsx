import { useTranslation } from "react-i18next";
import { GuidancePlanCard, QuestionAnswerSummary } from "@argus/ui";
import { useConversationInteractions } from "./use-conversation-interactions";
export function ConversationInteractionItem({
  conversationId,
  id,
}: {
  conversationId: string;
  id: string;
}) {
  const { t } = useTranslation(),
    { data } = useConversationInteractions(conversationId);
  const value = data?.items.find((item) => item.id === id);
  if (!value || value.status === "superseded") return null;
  if (value.kind === "plan")
    return (
      <GuidancePlanCard
        title={value.content.title ?? ""}
        markdown={
          value.content.document_markdown ??
          value.content.markdown ??
          legacyPlanMarkdown(value.content)
        }
        revision={value.revision}
        status={`${t("chatInteractions.revision", { revision: value.revision })} · ${t(`chatInteractions.${value.status}`)}`}
      />
    );
  if (value.status === "pending") return null;
  const questions = value.content.questions ?? [];
  return (
    <QuestionAnswerSummary
      label={t("chatInteractions.answeredCount", { count: questions.length })}
      items={questions.map((question) => {
        const answer = value.response?.answers?.[question.id];
        return {
          question: question.question,
          answer:
            !answer || answer.skipped
              ? t("chatInteractions.skipped")
              : [answer.selected_option, answer.text]
                  .filter(Boolean)
                  .join("\n"),
        };
      })}
    />
  );
}

function legacyPlanMarkdown(content: {
  title?: string;
  description?: string;
  steps?: {
    description: string;
    code?: string;
    language?: string;
    target?: string;
  }[];
}) {
  return (
    `# ${content.title ?? ""}\n\n${content.description ?? ""}\n\n` +
    (content.steps ?? [])
      .map((step) => {
        const runs = step.code?.match(/`+/g) ?? [];
        const fence = "`".repeat(
          Math.max(3, ...runs.map((run) => run.length + 1)),
        );
        const language = /^[a-zA-Z0-9_-]+$/.test(step.language ?? "")
          ? step.language
          : "text";
        return `${step.description}\n\n${step.target ?? ""}\n\n${step.code ? `${fence}${language}\n${step.code}\n${fence}` : ""}`;
      })
      .join("\n\n")
  );
}
