import { useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  ApiError,
  formatApiError,
  formatErrorCode,
  type PendingActionPublic,
} from "@argus/api-client";
import {
  CommandApproval,
  CommandExecution,
  CodeBlock,
  Skeleton,
} from "@argus/ui";
import { MfaStepUpDialog } from "../security/mfa-step-up-dialog";
import { RemoteContextDetails } from "./remote-context-details";

export function RemoteExecutionCard({
  action,
}: {
  action: PendingActionPublic;
}) {
  const api = useApi(),
    { t } = useTranslation(),
    cache = useQueryClient();
  const [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null),
    [stepUpOpen, setStepUpOpen] = useState(false);
  const pendingVerification = useRef<(() => Promise<unknown>) | null>(null);
  const preview = action.preview as Record<string, unknown>;
  const id = typeof preview?.id === "string" ? preview.id : "";
  const conversation =
    typeof preview?.conversation_id === "string" ? preview.conversation_id : "";
  const query = useQuery({
    queryKey: ["remote-execution", conversation, id],
    queryFn: () => api.remoteExecutions.get(conversation, id),
    enabled: !!id && !!conversation,
    refetchInterval: (q) =>
      ["awaiting_confirmation", "ready", "running", "stopping"].includes(
        q.state.data?.status ?? "",
      )
        ? 1000
        : false,
  });
  const refresh = async () => {
    await Promise.all([
      cache.invalidateQueries({
        queryKey: ["remote-execution", conversation, id],
      }),
      cache.invalidateQueries({ queryKey: ["approvals", action.action_ref] }),
      cache.invalidateQueries({
        queryKey: ["conversations", conversation, "events"],
      }),
    ]);
  };
  const perform = async (operation: () => Promise<unknown>, verify = false) => {
    setBusy(true);
    setError(null);
    try {
      await operation();
      await refresh();
    } catch (e) {
      if (verify && e instanceof ApiError && e.code === "STEP_UP_REQUIRED") {
        pendingVerification.current = operation;
        setStepUpOpen(true);
        return;
      }
      setError(
        formatApiError(e, t("plan7.failure"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      setBusy(false);
    }
  };
  if (!id || !conversation || query.isError)
    return <p role="alert">{t("plan7.failure")}</p>;
  const view = query.data;
  if (!view) return <Skeleton height={80} />;
  if (view.spec.kind === "plan" && view.status === "superseded") return null;
  const canConfirm =
    view.spec.kind !== "plan" &&
    view.status === "awaiting_confirmation" &&
    action.status === "awaiting_confirmation" &&
    (view.confirmation_required ?? true) &&
    Date.now() < Date.parse(action.expires_at);
  const stage =
    action.status === "awaiting_approval"
      ? "awaiting_approval"
      : view.status === "awaiting_confirmation"
        ? action.status === "cancelled" || action.status === "expired"
          ? action.status
          : action.status === "rejected"
            ? "failed"
            : Date.now() >= Date.parse(action.expires_at)
              ? "expired"
              : view.confirmation_required === false
                ? "waiting_policy"
                : view.status
        : view.status;
  const state = t(`plan7.states.${stage}`);
  const status =
    stage === "succeeded"
      ? "success"
      : [
            "failed",
            "partial_failed",
            "result_unknown",
            "paused_for_recovery",
            "cancelled",
            "expired",
          ].includes(stage)
        ? "failed"
        : "running";
  const content = (
    <>
      {view.spec.steps.map((step, index) => {
        const result = view.results[index];
        const target = [
          step.target.name ?? step.target.resource_id,
          step.target.namespace,
          step.target.pod,
          step.target.container,
        ]
          .filter(Boolean)
          .join(" / ");
        return (
          <div className="argus-command-activity__body" key={step.id}>
            <p className="argus-command-activity__target">
              {target} · {step.identity.username}
            </p>
            <CodeBlock
              code={step.script}
              language={step.interpreter}
              label={t("chatInteractions.command")}
            />
            <details>
              <summary>{t("plan7.context")}</summary>
              <RemoteContextDetails
                step={step}
                conversation={conversation}
                continuesPlan={false}
              />
            </details>
            {!canConfirm && (
              <>
                {result?.stdout && (
                  <CodeBlock
                    code={result.stdout}
                    label={t("chatInteractions.output")}
                  />
                )}
                {result?.stderr && (
                  <CodeBlock
                    code={result.stderr}
                    label={t("chatInteractions.errorOutput")}
                  />
                )}
                {result?.terminated && (
                  <p>
                    {t("chatInteractions.exitCode", { code: result.exit_code })}
                  </p>
                )}
                {result?.truncated && <p>{t("plan7.truncated")}</p>}
                {result?.terminated &&
                  !result.outputs_collected &&
                  (result.status === "collecting" ||
                    (result.status === "succeeded" &&
                      view.error_code ===
                        "REMOTE_OUTPUT_COLLECTION_FAILED")) && (
                    <p role="status">{t("plan7.outputsPending")}</p>
                  )}
                {!!result?.output_files.length && (
                  <details>
                    <summary>{t("plan7.outputs")}</summary>
                    <ul>
                      {result.output_files.map((file) => (
                        <li key={file.path}>
                          <code>{file.path}</code> · {file.byte_size} ·{" "}
                          <code>{file.content_hash}</code>
                        </li>
                      ))}
                    </ul>
                  </details>
                )}
              </>
            )}
          </div>
        );
      })}
      {!!view.spec.inputs.length && (
        <details>
          <summary>
            {t("plan7.inputs")} ({view.spec.inputs.length})
          </summary>
          <ul>
            {view.spec.inputs.map((file) => (
              <li key={file.path}>
                {file.id ? (
                  <a
                    href={api.remoteExecutions.inputUrl(
                      conversation,
                      id,
                      file.id,
                    )}
                    download={file.path}
                  >
                    {file.path}
                  </a>
                ) : (
                  <code>{file.path}</code>
                )}{" "}
                · {file.byte_size} · <code>{file.content_hash}</code>
                {file.content_preview !== undefined && (
                  <CodeBlock
                    code={file.content_preview}
                    label={t("chatInteractions.inputFile")}
                  />
                )}
              </li>
            ))}
          </ul>
        </details>
      )}
      {view.error_code && (
        <p role="status">{formatErrorCode(view.error_code, view.error_code)}</p>
      )}
      {view.stop_status === "unknown" && (
        <p role="status">{t("plan7.stopUnknown")}</p>
      )}
    </>
  );
  return (
    <>
      {canConfirm ? (
        <CommandApproval
          title={view.spec.title}
          summary={view.spec.description}
          busy={busy}
          onConfirm={() =>
            void perform(() => api.approvals.confirm(action.action_ref), true)
          }
          onCancel={() =>
            void perform(() => api.approvals.cancel(action.action_ref))
          }
          labels={{ confirm: t("plan7.confirm"), cancel: t("plan7.cancel") }}
        >
          {content}
        </CommandApproval>
      ) : (
        <CommandExecution
          title={t("chatInteractions.commandExecution")}
          summary={view.spec.steps[0]?.description ?? view.spec.title}
          status={status}
          state={state}
          labels={{
            expand: t("chatInteractions.expandExecution"),
            collapse: t("chatInteractions.collapseExecution"),
          }}
        >
          {content}
        </CommandExecution>
      )}
      <MfaStepUpDialog
        open={stepUpOpen}
        onOpenChange={setStepUpOpen}
        title={t("account.mfa.confirmActionTitle")}
        description={t("account.mfa.confirmActionDescription")}
        onComplete={async () => {
          const operation = pendingVerification.current;
          pendingVerification.current = null;
          if (operation) await perform(operation);
        }}
      />
      {error && <p role="alert">{error}</p>}
    </>
  );
}
