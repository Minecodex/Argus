import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  applicationAPMQueries,
  formatApiError,
  useApi,
} from "@argus/api-client";
import { Alert, Dialog, ObservationPanel, Spinner } from "@argus/ui";

export type ApplicationTraceSelection = {
  traceId: string;
  sourceId: string;
  range: { from: string; to: string };
};

export function ApplicationTraceDetail({
  applicationId,
  selected,
  onClose,
}: {
  applicationId: string;
  selected: ApplicationTraceSelection;
  onClose: () => void;
}) {
  const api = useApi(),
    { t } = useTranslation();
  const query = useQuery({
    queryKey: ["application-trace-detail", applicationId, selected],
    retry: false,
    queryFn: () =>
      api.telemetry.queryTraces({
        query: applicationAPMQueries.trace_detail,
        operation_name: "ApplicationAPM",
        variables: {
          traceId: selected.traceId,
          sourceId: selected.sourceId,
          resourceId: applicationId,
        },
        resource_ids: [applicationId],
        time_range: selected.range,
        budget: {
          max_samples: 100000,
          max_series: 10000,
          max_rows: 5000,
          max_scan_bytes: 268435456,
          max_result_bytes: 8388608,
          timeout_ms: 10000,
        },
      }),
  });
  return (
    <Dialog
      open
      size="cover"
      title={t("applicationObservability.traceDetail")}
      description={selected.traceId}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      {query.isFetching && <Spinner label={t("common.loading")} />}
      {query.error && (
        <Alert
          tone="danger"
          title={t("telemetry.queryFailed")}
          description={formatApiError(
            query.error,
            t("monitoring.errors.unavailable"),
            (id) => t("common.requestReference", { requestId: id }),
          )}
        />
      )}
      {query.data && (
        <ObservationPanel
          title={t("applicationObservability.traceDetail")}
          type="trace_detail"
          signal="traces"
          range={selected.range}
          targets={[
            {
              id: "trace",
              status: query.data.extensions.argus.partial
                ? "partial"
                : "success",
              result_type: "traces",
              data: query.data.data,
            },
          ]}
        />
      )}
    </Dialog>
  );
}
