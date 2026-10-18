import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useApi, type RemoteExecution } from "@argus/api-client";
import { CodeBlock, Skeleton } from "@argus/ui";

export function RemoteContextDetails({
  step,
  conversation,
  continuesPlan,
}: {
  step: RemoteExecution["spec"]["steps"][number];
  conversation: string;
  continuesPlan: boolean;
}) {
  const api = useApi();
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const previous = useQuery({
    queryKey: ["remote-execution", conversation, step.previous_request_id],
    queryFn: () =>
      api.remoteExecutions.get(conversation, step.previous_request_id!),
    enabled: open && !!step.previous_request_id,
  });
  const index = (step.previous_sequence ?? 0) - 1;
  const prior = previous.data?.spec.steps[index];
  const matching =
    prior?.session_id === step.session_id &&
    prior?.session_generation === step.session_generation;
  return (
    <>
      <p>
        {t("plan7.contextLocation")}:{" "}
        <code>{step.known_directory || t("plan7.contextUnobserved")}</code>
      </p>
      <p>
        {t("plan7.contextVersion")}:{" "}
        <code>{step.interpreter_version || t("plan7.contextUnobserved")}</code>
      </p>
      <p>
        {t("plan7.contextIdentity")}:{" "}
        <code>
          {step.session_id} · {step.session_generation} ·{" "}
          {step.context_revision}
        </code>
      </p>
      {continuesPlan && <p>{t("plan7.contextContinues")}</p>}
      {step.previous_request_id && (
        <details onToggle={(event) => setOpen(event.currentTarget.open)}>
          <summary>
            {t("plan7.previousBatch", { sequence: step.previous_sequence })}
          </summary>
          {open &&
            (previous.isError ? (
              <p role="alert">{t("plan7.previousUnavailable")}</p>
            ) : !previous.data ? (
              <Skeleton height={60} />
            ) : matching && prior ? (
              <>
                <p>
                  {previous.data.spec.title} ·{" "}
                  {t(`plan7.states.${previous.data.status}`)} ·{" "}
                  {t(
                    `plan7.states.${previous.data.results[index]?.status ?? previous.data.status}`,
                  )}
                </p>
                <CodeBlock code={prior.script} language={prior.interpreter} />
              </>
            ) : (
              <p role="alert">{t("plan7.previousUnavailable")}</p>
            ))}
        </details>
      )}
    </>
  );
}
