import type { AlertInstance } from "@argus/api-client";
import { DataTable, type Column } from "@argus/ui";
import { AlertCallout, datetime, useAlertText } from "./common";

type Report = NonNullable<AlertInstance["state"]["delivery_gap"]>;
type CounterRow = {
  id: string;
  signal: string;
  terminal: number;
  refused: number;
  enqueue: number;
};

export function CollectorDeliveryGap({ report }: { report: Report }) {
  const { t, locale } = useAlertText();
  const rows: CounterRow[] = ["log_records", "spans", "metric_points"].map(
    (key) => {
      const signal = key as keyof Report["terminal_failures"];
      return {
        id: key,
        signal: t(`alerts.deliverySignal_${key}`),
        terminal: report.terminal_failures[signal],
        refused: report.receiver_refusals[signal],
        enqueue: report.enqueue_failures[signal],
      };
    },
  );
  const columns: Column<CounterRow>[] = [
    {
      key: "signal",
      header: t("alerts.deliverySignal"),
      render: (row) => row.signal,
    },
    {
      key: "terminal",
      header: t("alerts.deliveryTerminal"),
      render: (row) => row.terminal.toLocaleString(locale),
    },
    {
      key: "refused",
      header: t("alerts.deliveryRefused"),
      render: (row) => row.refused.toLocaleString(locale),
    },
    {
      key: "enqueue",
      header: t("alerts.deliveryEnqueue"),
      render: (row) => row.enqueue.toLocaleString(locale),
    },
  ];
  const facts = [
    ["deliveryObservedSince", datetime(report.observed_since)],
    ["deliveryObservedAt", datetime(report.observed_at)],
    ["deliveryLastFailure", datetime(report.last_failure_at)],
    [
      "deliveryRetryLimit",
      t("alerts.deliveryRetrySeconds", { seconds: report.retry_limit_seconds }),
    ],
    ["deliveryAttribution", t("alerts.deliveryTargetsUnknown")],
  ];
  return (
    <section
      className="argus-alert-drawer-section"
      data-testid="collector-delivery-gap"
    >
      <h3>{t("alerts.deliveryGapTitle")}</h3>
      <AlertCallout danger>{t("alerts.deliveryGapExplanation")}</AlertCallout>
      <DataTable
        density="compact"
        surface="outlined"
        tableClassName="argus-alert-table"
        data={rows}
        columns={columns}
        getRowKey={(row) => row.id}
      />
      <div className="argus-alert-detail-grid">
        {facts.map(([key, value]) => (
          <div key={key}>
            <span className="argus-alert-secondary">{t(`alerts.${key}`)}</span>
            <div className="argus-alert-detail-value">{value}</div>
          </div>
        ))}
      </div>
      <p className="argus-alert-secondary">{t("alerts.deliveryGapHistory")}</p>
      <p className="argus-alert-secondary">
        {t("alerts.deliveryObservationCoverage")}
      </p>
    </section>
  );
}
