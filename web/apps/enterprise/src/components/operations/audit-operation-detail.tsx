import { useInfiniteQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useApi, formatErrorCode, type AuditEvent } from "@argus/api-client";
import { Alert, Button, OperationPanel, Spinner, Timeline } from "@argus/ui";
import { formatDateTime } from "../settings/shared";
import { operationActive, operationTone } from "./execution-result";

export function AuditOperationDetail({ event }: { event: AuditEvent }) {
  const { t, i18n } = useTranslation();
  const api = useApi();
  const ref = event.operation?.action_ref;
  const query = useInfiniteQuery({
    queryKey: ["audit", "operation", ref],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      api.audit.list(
        { operationRef: ref },
        { page: { limit: 50, cursor: pageParam } },
      ),
    getNextPageParam: (page) =>
      page.hasMore ? (page.nextCursor ?? undefined) : undefined,
    enabled: Boolean(ref),
    refetchInterval: (q) => {
      const operation =
        q.state.data?.pages[0]?.items[0]?.operation ?? event.operation;
      return operation &&
        operationActive(operation.execution_status ?? operation.action_status)
        ? 2000
        : false;
    },
  });
  const events = [
    ...new Map(
      (query.data?.pages.flatMap((page) => page.items) ?? []).map((item) => [
        item.id,
        item,
      ]),
    ).values(),
  ].sort(
    (a, b) =>
      a.createdAt.localeCompare(b.createdAt) || a.id.localeCompare(b.id),
  );
  const operation =
    query.data?.pages[0]?.items[0]?.operation ?? event.operation;
  if (!operation) return null;
  const status = operation.execution_status ?? operation.action_status;
  const label = (code: string) =>
    t(`settings.audit.actions.${code.replaceAll(".", "_")}`, {
      defaultValue: t("operations.unknownAction"),
    });
  return (
    <div className="argus-operation-list">
      <OperationPanel
        title={label(operation.action_type)}
        description={
          operation.resource_name ??
          t(`operations.hints.${status}`, {
            defaultValue: t("operations.hints.unavailable"),
          })
        }
        statusLabel={t(`operations.status.${status}`, {
          defaultValue: t("operations.status.unavailable"),
        })}
        tone={operationTone(status)}
        busy={operationActive(status)}
        meta={
          <>
            <span>
              {t("operations.created", {
                time: formatDateTime(operation.created_at, i18n.language),
              })}
            </span>
            <span>
              {t("operations.updated", {
                time: formatDateTime(operation.updated_at, i18n.language),
              })}
            </span>
          </>
        }
      >
        <p className="argus-muted">{t("operations.stateHint")}</p>
        {operation.error_code && (
          <Alert
            tone="danger"
            title={t("operations.status.failed")}
            description={formatErrorCode(
              operation.error_code,
              t("operations.hints.failed"),
            )}
          />
        )}
      </OperationPanel>
      <section aria-label={t("operations.timeline")}>
        <h3>{t("operations.timeline")}</h3>
        {query.isPending ? (
          <Spinner label={t("common.loading")} />
        ) : query.isError ? (
          <Alert
            tone="warning"
            title={t("settings.audit.loadFailed")}
            description={t("operations.retryHint")}
          />
        ) : events.length ? (
          <Timeline
            items={events.map((item) => ({
              title: label(item.action),
              status: item.result === "success" ? "done" : "danger",
              meta: `${formatDateTime(item.createdAt, i18n.language)} · ${item.actorName !== item.actorUserId ? item.actorName : t(`settings.audit.actorTypes.${item.actorType}`)} · ${t(`settings.audit.results.${item.result}`)}`,
            }))}
          />
        ) : (
          <p className="argus-muted">{t("operations.noEvents")}</p>
        )}
        {query.hasNextPage && (
          <Button
            variant="secondary"
            isPending={query.isFetchingNextPage}
            onPress={() => void query.fetchNextPage()}
          >
            {t("operations.moreEvents")}
          </Button>
        )}
      </section>
    </div>
  );
}
