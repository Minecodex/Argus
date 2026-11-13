import { useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  formatErrorCode,
  type ConfirmActionResult,
  type KubernetesResourceReference,
} from "@argus/api-client";
import { Alert, Badge, Button } from "@argus/ui";
import { confirmedKubernetesOperation } from "./confirmation-operation";
import { operationResource } from "./operation-result";
const terminal = new Set([
  "succeeded",
  "failed",
  "cancelled",
  "expired",
  "conflict",
  "rejected",
  "invalidated",
]);
export function KubernetesOperationFeedback({
  clusterId,
  submission,
  onSettled,
  onDetails,
  onReview,
  onOpen,
}: {
  clusterId: string;
  submission: ConfirmActionResult;
  onSettled: () => void;
  onDetails: (operationId: string) => void;
  onOpen?: (reference: KubernetesResourceReference) => void;
  onReview?: () => void;
}) {
  const api = useApi(),
    { t } = useTranslation(),
    notified = useRef(false),
    started = useRef(Date.now()),
    operation = useRef(confirmedKubernetesOperation(submission)),
    execution = useRef(submission.execution?.execution_id);
  const query = useQuery({
    queryKey: [
      "kubernetes-workbench",
      clusterId,
      "submitted",
      submission.pending_action.action_ref,
    ],
    queryFn: async () => {
      if (operation.current)
        return api.kubernetesWorkbench.operation(clusterId, operation.current);
      if (!execution.current) {
        const page = await api.executions.list({
          actionRef: submission.pending_action.action_ref,
        });
        execution.current = page.items[0]?.execution_id;
      }
      if (execution.current) {
        const row = await api.executions.get(execution.current);
        operation.current = confirmedKubernetesOperation({
          ...submission,
          execution: row,
        });
        if (operation.current)
          return api.kubernetesWorkbench.operation(
            clusterId,
            operation.current,
          );
        return {
          name: submission.pending_action.title,
          status: row.status,
          error_code: row.error_code,
        };
      }
      const action = await api.approvals.get(
        submission.pending_action.action_ref,
      );
      return {
        name: action.title,
        status: action.status,
        error_code: undefined,
      };
    },
    refetchInterval: (q) =>
      (q.state.data && terminal.has(q.state.data.status)) ||
      Date.now() - started.current > 300000
        ? false
        : 2000,
    refetchOnWindowFocus: false,
    retry: false,
  });
  useEffect(() => {
    if (query.data && terminal.has(query.data.status) && !notified.current) {
      notified.current = true;
      onSettled();
    }
  }, [query.data, onSettled]);
  const state =
    query.data?.status ??
    submission.execution?.status ??
    submission.pending_action.status;
  const resultResource =
    query.data && "result" in query.data
      ? operationResource(query.data)
      : undefined;
  const labels: Record<string, string> = {
    pending: "queued",
    ready: "queued",
    executing: "running",
    awaiting_approval: "awaitingApproval",
    invalidated: "failed",
    rejected: "rejected",
    expired: "expired",
  };
  return (
    <section
      className="argus-k8s-operation-feedback"
      role="status"
      aria-label={t("kubernetesResources.operationProgress")}
    >
      <strong>{query.data?.name ?? submission.pending_action.title}</strong>
      <Badge
        tone={
          state === "succeeded"
            ? "success"
            : state === "failed"
              ? "danger"
              : "neutral"
        }
      >
        {t(
          query.data?.error_code === "KUBERNETES_OPERATION_SUPERSEDED"
            ? "kubernetesResources.superseded"
            : "kubernetesWorkbench.mutation.statuses." +
                (labels[state] ?? state),
          { defaultValue: state },
        )}
      </Badge>
      {query.data?.error_code && (
        <Alert
          tone="warning"
          title={formatErrorCode(
            query.data.error_code,
            t("kubernetesWorkbench.errors.unavailable"),
          )}
          description=""
        />
      )}
      {query.error && <p>{t("kubernetesWorkbench.errors.unavailable")}</p>}
      <Button variant="ghost" onPress={() => void query.refetch()}>
        {t("common.refresh")}
      </Button>
      {onReview &&
        ["failed", "invalidated", "rejected", "cancelled", "expired"].includes(
          state,
        ) && (
          <Button onPress={onReview}>
            {t("kubernetesResources.reviewInput")}
          </Button>
        )}
      {resultResource && onOpen && (
        <Button variant="ghost" onPress={() => onOpen(resultResource)}>
          {t("kubernetesResources.openResult")}
        </Button>
      )}
      <Button
        variant="ghost"
        isDisabled={!operation.current}
        onPress={() => operation.current && onDetails(operation.current)}
      >
        {t("kubernetesResources.operationDetails")}
      </Button>
    </section>
  );
}
