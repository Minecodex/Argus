import { OperationalPanelDetails } from "./operational-panel-details";
import { OperationalPanel } from "./operational-panel";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  formatApiError,
  type DashboardExecution,
  type DashboardPanel,
  type DashboardSchemas,
} from "@argus/api-client";
import {
  Alert,
  Button,
  Dialog,
  ObservationDetails,
  ObservationPanel,
  Spinner,
} from "@argus/ui";
import { queryScopeBounds } from "./query-scope";
import { TimeRangePicker } from "./time-range-picker";

type Range = DashboardSchemas["DashboardTimeRange"];
type Bounds = ReturnType<typeof queryScopeBounds>;

export function ExpandedPanel({
  panel,
  revisionId,
  initialTime,
  initialBounds,
  initialExecution,
  selectTime,
  execute,
  onClose,
}: {
  panel: DashboardPanel;
  revisionId: string;
  initialTime: Range;
  initialBounds?: Bounds;
  initialExecution?: DashboardSchemas["DashboardPanelExecution"];
  selectTime: boolean;
  execute?: (
    bounds: Bounds,
    signal: AbortSignal,
  ) => Promise<DashboardExecution>;
  onClose: () => void;
}) {
  const { t, i18n } = useTranslation();
  const [time, setTime] = useState(initialTime);
  const [request, setRequest] = useState<{
    id: string;
    range: Range;
    bounds: Bounds;
  }>();
  const query = useQuery({
    queryKey: ["dashboard-expanded-panel", request?.id],
    queryFn: ({ signal }) => execute!(request!.bounds, signal),
    enabled: Boolean(request && execute),
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const revisionChanged = query.data && query.data.revision_id !== revisionId;
  const result = !revisionChanged
    ? query.data?.panels.find((item) => item.id === panel.id)
    : undefined;
  const targets = (request ? result : initialExecution)?.targets ?? [];
  const failed = query.error || (query.isSuccess && !result);
  const shownBounds = result ? query.data : request ? undefined : initialBounds;
  const formatTime = new Intl.DateTimeFormat(i18n.language, {
    dateStyle: "short",
    timeStyle: "medium",
  });

  return (
    <Dialog
      open
      onOpenChange={(open) => !open && onClose()}
      title={panel.title}
      titleExtra={
        panel.data_source && panel.data_source.kind !== "telemetry" ? (
          <OperationalPanelDetails
            title={panel.title}
            execution={request ? result : initialExecution}
          />
        ) : (
          <ObservationDetails
            title={panel.title}
            targets={targets}
            signal={panel.signal}
            display={panel.display}
          />
        )
      }
      width={1200}
    >
      <div className="argus-dashboard-form-stack">
        {execute && (
          <div className="argus-dashboard-inline">
            {selectTime && (
              <TimeRangePicker
                label={t("dashboards.time")}
                value={time}
                onChange={setTime}
              />
            )}
            <Button
              variant="primary"
              isPending={query.isFetching}
              onPress={() =>
                setRequest({
                  id: crypto.randomUUID(),
                  range: time,
                  bounds: queryScopeBounds(time),
                })
              }
            >
              {t("dashboards.editor.run")}
            </Button>
            {time !== (request?.range ?? initialTime) && (
              <span className="argus-dashboard-muted" role="status">
                {t("dashboards.expandedTimePending")}
              </span>
            )}
          </div>
        )}
        {shownBounds && (
          <div className="argus-dashboard-inline argus-dashboard-muted">
            {t("dashboards.expandedResultRange")}
            <time dateTime={shownBounds.from}>
              {formatTime.format(new Date(shownBounds.from))}
            </time>
            <span aria-hidden>—</span>
            <time dateTime={shownBounds.to}>
              {formatTime.format(new Date(shownBounds.to))}
            </time>
          </div>
        )}
        <div className="argus-dashboard-detail-content">
          {query.isFetching ? (
            <Spinner label={t("common.loading")} />
          ) : failed ? (
            <Alert
              tone="danger"
              title={t("dashboards.failed")}
              description={
                revisionChanged
                  ? t("dashboards.expandedRevisionChanged")
                  : formatApiError(query.error, t("dashboards.failed"), (id) =>
                      t("common.requestReference", { requestId: id }),
                    )
              }
            />
          ) : panel.data_source && panel.data_source.kind !== "telemetry" ? (
            <OperationalPanel
              panel={panel}
              execution={request ? result : initialExecution}
            />
          ) : (
            <ObservationPanel
              title={panel.title}
              details="external"
              type={panel.type}
              signal={panel.signal}
              unit={panel.unit}
              decimals={panel.decimals}
              legend={panel.legend}
              display={panel.display}
              thresholds={panel.thresholds}
              targets={targets}
            />
          )}
        </div>
      </div>
    </Dialog>
  );
}
