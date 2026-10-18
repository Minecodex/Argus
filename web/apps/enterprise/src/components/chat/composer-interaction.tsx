import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button, ComposerPanel, QuestionChoices, Textarea } from "@argus/ui";
import {
  formatApiError,
  type ConversationInteraction,
  type InteractionAnswer,
  type InteractionResponse,
} from "@argus/api-client";
export function ComposerInteraction({
  interaction,
  onRespond,
  onClose,
}: {
  interaction: ConversationInteraction;
  onRespond(id: string, value: InteractionResponse): Promise<void>;
  onClose(): void;
}) {
  const { t } = useTranslation();
  const [index, setIndex] = useState(0),
    [feedback, setFeedback] = useState("");
  const [answers, setAnswers] = useState<Record<string, InteractionAnswer>>({});
  const [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null);
  const questions = interaction.content.questions ?? [],
    question = questions[index];
  const isPlan = interaction.kind === "plan";
  const answer = question ? (answers[question.id] ?? {}) : {};
  const filled = (a?: InteractionAnswer) =>
    !!(a?.skipped || a?.selected_option || a?.text?.trim());
  const allAnswered = questions.every((q) => filled(answers[q.id]));
  const submit = async (
    action: InteractionResponse["action"],
    values = answers,
  ) => {
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      await onRespond(interaction.id, {
        version: interaction.version,
        content_hash: interaction.content_hash,
        action,
        ...(isPlan
          ? feedback.trim()
            ? { feedback: feedback.trim() }
            : {}
          : { answers: values }),
      });
    } catch (e) {
      setError(
        formatApiError(e, t("chatInteractions.failure"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      setBusy(false);
    }
  };
  const skip = () => {
    if (!question) return;
    const values = { ...answers, [question.id]: { skipped: true } };
    setAnswers(values);
    if (questions.every((q) => filled(values[q.id])))
      void submit("answer", values);
    else setIndex(Math.min(index + 1, questions.length - 1));
  };
  const advance = () => {
    if (isPlan) void submit(feedback.trim() ? "revise" : "confirm");
    else if (allAnswered) void submit("answer");
    else if (filled(answer))
      setIndex(Math.min(index + 1, questions.length - 1));
  };
  return (
    <ComposerPanel
      title={
        isPlan
          ? t("chatInteractions.implementQuestion")
          : (question?.question ?? "")
      }
      progress={
        !isPlan && questions.length > 1
          ? t("chatInteractions.questionProgress", {
              current: index + 1,
              total: questions.length,
            })
          : undefined
      }
      onPrevious={!busy && index > 0 ? () => setIndex(index - 1) : undefined}
      onNext={
        !busy && index < questions.length - 1
          ? () => setIndex(index + 1)
          : undefined
      }
      onClose={onClose}
      labels={{
        close: t("chatInteractions.close"),
        previous: t("chatInteractions.previous"),
        next: t("chatInteractions.next"),
      }}
      footer={
        <>
          {!isPlan && (
            <Button variant="ghost" size="sm" isDisabled={busy} onPress={skip}>
              {t("chatInteractions.skip")}
            </Button>
          )}
          <Button
            size="sm"
            isDisabled={
              busy ||
              (!isPlan &&
                !allAnswered &&
                (!filled(answer) || index === questions.length - 1))
            }
            onPress={advance}
          >
            {busy
              ? t("chatInteractions.saving")
              : isPlan
                ? t(
                    feedback.trim()
                      ? "chatInteractions.revise"
                      : "chatInteractions.implement",
                  )
                : t(
                    allAnswered || index === questions.length - 1
                      ? "chatInteractions.submit"
                      : "chatInteractions.next",
                  )}
          </Button>
        </>
      }
    >
      {isPlan ? (
        <p className="argus-chat-interaction__hint">
          {t("chatInteractions.planGuidance")}
        </p>
      ) : (
        question && (
          <QuestionChoices
            name={`${interaction.id}-${question.id}`}
            label={question.question}
            options={question.options}
            selected={answer.selected_option}
            disabled={busy}
            onSelect={(selected_option) =>
              setAnswers((current) => ({
                ...current,
                [question.id]: {
                  ...current[question.id],
                  selected_option,
                  skipped: false,
                },
              }))
            }
          />
        )
      )}
      <Textarea
        appearance="embedded"
        aria-label={t(
          isPlan ? "chatInteractions.feedback" : "chatInteractions.answer",
        )}
        rows={2}
        disabled={busy}
        value={isPlan ? feedback : (answer.text ?? "")}
        placeholder={t(
          isPlan
            ? "chatInteractions.feedbackPlaceholder"
            : "chatInteractions.answerPlaceholder",
        )}
        maxLength={16384}
        onChange={(event) =>
          isPlan
            ? setFeedback(event.target.value)
            : question &&
              setAnswers((current) => ({
                ...current,
                [question.id]: {
                  ...current[question.id],
                  text: event.target.value,
                  skipped: false,
                },
              }))
        }
        onKeyDown={(event) => {
          if (
            event.key === "Enter" &&
            !event.shiftKey &&
            !event.nativeEvent.isComposing &&
            (isPlan || allAnswered)
          ) {
            event.preventDefault();
            advance();
          }
        }}
      />
      {error && <p role="alert">{error}</p>}
    </ComposerPanel>
  );
}
