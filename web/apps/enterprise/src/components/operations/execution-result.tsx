import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link } from "@tanstack/react-router";
import { Download, RefreshCw } from "lucide-react";
import {
  useApi,
  formatApiError,
  formatErrorCode,
  type ActionOneTimeResult,
  type Execution,
  type PendingActionPublic,
} from "@argus/api-client";
import { Alert, Button, EmptyState, OperationPanel, Spinner } from "@argus/ui";
import { InstallInstructionPanel } from "../hosts/install-instruction-panel";
import { formatDateTime } from "../settings/shared";
import { presentPendingAction } from "../pending-action-presentation";
import { usePermission } from "../../lib/permissions";

export function operationTone(status: string) {
  if (status === "succeeded") return "success" as const;
  if (["failed", "rejected", "invalidated"].includes(status))
    return "danger" as const;
  if (["result_unknown", "awaiting_approval", "expired"].includes(status))
    return "warning" as const;
  if (["running", "executing", "ready", "pending"].includes(status))
    return "info" as const;
  return "neutral" as const;
}
export const operationActive = (status: string) =>
  [
    "pending",
    "running",
    "ready",
    "executing",
    "result_unknown",
    "awaiting_approval",
  ].includes(status);

export function ExecutionResult({
  execution,
  title,
  resourceName,
  summary,
  onRefresh,
}: {
  execution: Execution;
  title: string;
  resourceName?: string;
  summary?: string;
  onRefresh?: () => void;
}) {
  const { t, i18n } = useTranslation();
  const api = useApi();
  const cache = useQueryClient();
  const canReadAudit = usePermission("audit.read");
  const [result, setResult] = useState<ActionOneTimeResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const claim = async () => {
    if (busy) return;
    setBusy(true);
    setError(undefined);
    try {
      setResult(
        await api.executions.claimOneTimeResult(execution.execution_id),
      );
      void cache.invalidateQueries({ queryKey: ["executions"] });
      void cache.invalidateQueries({ queryKey: ["hosts"] });
    } catch (cause) {
      setError(
        formatApiError(cause, t("operations.claimFailed"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      setBusy(false);
    }
  };
  const state = execution.one_time_result_state;
  return (
    <OperationPanel
      title={title}
      description={resourceName || t(`operations.hints.${execution.status}`)}
      statusLabel={t(`operations.status.${execution.status}`)}
      tone={operationTone(execution.status)}
      busy={operationActive(execution.status)}
      meta={
        <>
          <span>
            {t("operations.created", {
              time: formatDateTime(execution.created_at, i18n.language),
            })}
          </span>
          <span>
            {t("operations.updated", {
              time: formatDateTime(execution.updated_at, i18n.language),
            })}
          </span>
        </>
      }
      actions={
        <>
          {state === "available" && !result && (
            <Button isPending={busy} onPress={() => void claim()}>
              <Download aria-hidden size={16} />
              {t("operations.claim")}
            </Button>
          )}
          {onRefresh && (
            <Button variant="secondary" onPress={onRefresh}>
              <RefreshCw aria-hidden size={16} />
              {t("operations.refresh")}
            </Button>
          )}
          {result && (
            <Button variant="secondary" onPress={() => setResult(null)}>
              {t("operations.dismiss")}
            </Button>
          )}
          {canReadAudit && (
            <Link
              to="/settings/audit"
              search={{ operation_ref: execution.action_ref }}
            >
              <Button variant="secondary">{t("operations.audit")}</Button>
            </Link>
          )}
        </>
      }
    >
      {summary && execution.status === "succeeded" && <p>{summary}</p>}
      {execution.error_code && (
        <Alert
          tone="danger"
          title={t("operations.status.failed")}
          description={formatErrorCode(
            execution.error_code,
            t("operations.hints.failed"),
          )}
        />
      )}
      {state === "available" && !result && (
        <p className="argus-muted">{t("operations.claimHint")}</p>
      )}
      {(state === "consumed" || state === "expired") && !result && (
        <Alert
          tone={state === "expired" ? "warning" : "info"}
          title={t(`operations.${state}`)}
          description={t(`operations.${state}Hint`)}
        />
      )}
      {error && (
        <Alert
          tone="danger"
          title={t("operations.claimFailed")}
          description={error}
        />
      )}
      {result && <InstallInstructionPanel result={result} />}
    </OperationPanel>
  );
}

export function ActionExecutionStatus({
  action,
}: {
  action: PendingActionPublic;
}) {
  const { t } = useTranslation();
  const api = useApi();
  const query = useQuery({
    queryKey: ["executions", "action", action.action_ref],
    queryFn: () => api.executions.list({ actionRef: action.action_ref }),
    refetchInterval: (q) =>
      (!q.state.data?.items.length && operationActive(action.status)) ||
      q.state.data?.items.some((e) => operationActive(e.status))
        ? 2000
        : false,
  });
  const presented = presentPendingAction(action, t);
  const execution = query.data?.items[0];
  if (query.isError)
    return (
      <Alert
        tone="warning"
        title={t("operations.loadFailed")}
        description={t("operations.retryHint")}
      />
    );
  if (!execution)
    return (
      <OperationPanel
        title={presented.title}
        description={t(`operations.hints.${action.status}`, {
          defaultValue: t("operations.hints.unavailable"),
        })}
        statusLabel={t(`operations.status.${action.status}`, {
          defaultValue: t("operations.status.unavailable"),
        })}
        tone={operationTone(action.status)}
        busy={operationActive(action.status)}
      />
    );
  return (
    <ExecutionResult
      key={execution.execution_id}
      execution={execution}
      title={presented.title}
      resourceName={presented.affectedName}
      summary={presented.resultSummary}
      onRefresh={() => void query.refetch()}
    />
  );
}

export function ResourceExecutionActivity({
  resourceId,
  resourceName,
}: {
  resourceId: string;
  resourceName: string;
}) {
  const { t } = useTranslation();
  const api = useApi();
  const allowed = usePermission("execution.read");
  const query = useQuery({
    queryKey: ["executions", "resource", resourceId],
    queryFn: () => api.executions.list({ resourceId }),
    enabled: allowed,
    refetchInterval: (q) =>
      q.state.data?.items.some((e) => operationActive(e.status)) ? 2000 : 15000,
  });
  if (!allowed) return null;
  return (
    <section
      className="argus-operation-list"
      aria-label={t("operations.title")}
    >
      <h3>{t("operations.title")}</h3>
      {query.isPending ? (
        <Spinner label={t("common.loading")} />
      ) : query.isError ? (
        <Alert
          tone="warning"
          title={t("operations.loadFailed")}
          description={t("operations.retryHint")}
        />
      ) : !query.data?.items.length ? (
        <EmptyState
          title={t("operations.empty")}
          description={t("operations.emptyHint")}
        />
      ) : (
        query.data.items
          .filter(
            (e, i) =>
              operationActive(e.status) ||
              e.one_time_result_state !== "unavailable" ||
              i < 3,
          )
          .map((execution) => (
            <ExecutionResult
              key={execution.execution_id}
              execution={execution}
              resourceName={resourceName}
              title={t(
                `settings.audit.actions.${(execution.action_type ?? "").replaceAll(".", "_")}`,
                { defaultValue: t("operations.generic") },
              )}
              onRefresh={() => void query.refetch()}
            />
          ))
      )}
    </section>
  );
}
