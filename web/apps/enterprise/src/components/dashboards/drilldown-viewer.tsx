import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  formatApiError,
  useApi,
  type DashboardPanel,
  type DashboardSchemas,
} from "@argus/api-client";
import { Alert, Button, Dialog, ObservationPanel } from "@argus/ui";
import { scalarPointer } from "./model";
import { SourceSummary } from "./source-summary";

export function DrilldownViewer({
  dashboardId,
  panel,
  token,
  initialRow,
  initialTarget,
  onClose,
}: {
  dashboardId: string;
  panel: DashboardPanel;
  token: string;
  initialRow: Record<string, unknown>;
  initialTarget: string;
  onClose: () => void;
}) {
  const api = useApi(),
    { t } = useTranslation();
  const [steps, setSteps] = useState<
      DashboardSchemas["DashboardDrilldownExecution"][]
    >([]),
    [selected, setSelected] = useState({
      row: initialRow,
      target: initialTarget,
    }),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const [origins, setOrigins] = useState<Array<typeof selected>>([]);
  const current = steps.at(-1);
  const target =
    current &&
    panel.detail_query_targets.find(
      (target) => target.id === current.result.id,
    );
  const actions = panel.drilldowns.filter(
    (d) =>
      d.origin_query_ref === selected.target &&
      Object.values(d.inputs).every(
        (path) => scalarPointer(selected.row, path) !== undefined,
      ),
  );
  const execute = async (d: DashboardSchemas["DashboardDrilldown"]) => {
    setBusy(true);
    setError("");
    try {
      const values = Object.fromEntries(
        Object.entries(d.inputs).map(([key, pointer]) => [
          key,
          scalarPointer(selected.row, pointer)!,
        ]),
      );
      const result = await api.dashboards.drilldown(dashboardId, {
        context_token: current?.context_token ?? token,
        panel_id: panel.id,
        drilldown_id: d.id,
        values,
        expand_authorized_resources: d.scope_policy === "authorized_trace",
      });
      setSteps([...steps, result]);
      setOrigins([...origins, selected]);
      setSelected({ row: {}, target: result.result.id });
    } catch (e) {
      setError(
        formatApiError(e, t("dashboards.failed"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      setBusy(false);
    }
  };
  const signal = target?.signal ?? panel.signal,
    kind = current?.result.result_type;
  const type =
    kind === "trace_graph"
      ? "trace_detail"
      : kind === "traces"
        ? "trace_list"
        : kind?.startsWith("apm_")
          ? kind
          : signal === "logs"
            ? kind === "log_entries"
              ? "logs"
              : "table"
            : panel.type;
  return (
    <Dialog
      open
      onOpenChange={(open) => !open && onClose()}
      title={`${panel.title} · ${t("dashboards.details")}`}
      width={1200}
      className="argus-dashboard-detail-dialog"
    >
      <div className="argus-dashboard-form-stack">
        {error && (
          <Alert
            tone="danger"
            title={t("dashboards.failed")}
            description={error}
          />
        )}
        <div className="argus-dashboard-inline">
          {steps.length > 0 && (
            <Button
              disabled={busy}
              onClick={() => {
                setSteps(steps.slice(0, -1));
                setSelected(origins.at(-1)!);
                setOrigins(origins.slice(0, -1));
                setError("");
              }}
            >
              {t("dashboards.previousDetail")}
            </Button>
          )}
          {actions.map((d) => (
            <Button
              key={d.id}
              disabled={busy}
              variant={
                d.scope_policy === "authorized_trace" ? "primary" : "secondary"
              }
              onClick={() => void execute(d)}
            >
              {d.title ||
                (d.scope_policy === "authorized_trace"
                  ? t("dashboards.fullTrace")
                  : d.kind === "inspect"
                    ? t("dashboards.expand")
                    : t(`dashboards.drillKinds.${d.kind}`, {
                        defaultValue: t("dashboards.details"),
                      }))}
            </Button>
          ))}
          {!actions.length && (
            <p className="argus-dashboard-muted">
              {t("dashboards.noDrilldown")}
            </p>
          )}
        </div>
        {actions.some((d) => d.scope_policy === "authorized_trace") && (
          <p className="argus-dashboard-muted">{t("dashboards.fullScope")}</p>
        )}
        {current && (
          <>
            <SourceSummary title={panel.title} sources={current.sources} />
            <p className="argus-dashboard-muted">
              {t("dashboards.inspectedScope")}:{" "}
              {current.resources.map((r) => r.id).join(", ")} · {current.from} —{" "}
              {current.to}
            </p>
            <div className="argus-dashboard-detail-content">
              <ObservationPanel
                title={panel.title}
                signal={signal}
                type={type}
                targets={[current.result]}
                unit={panel.unit}
                decimals={panel.decimals}
                onSelect={(row, target) => setSelected({ row, target })}
              />
            </div>
          </>
        )}
      </div>
    </Dialog>
  );
}
