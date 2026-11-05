import { useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type {
  DashboardSchemas,
  MonitoringApplication,
  MonitoringService,
} from "@argus/api-client";
import {
  Alert,
  Button,
  Input,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@argus/ui";
import { queryScopeBounds } from "../dashboards/query-scope";
import { TimeRangePicker } from "../dashboards/time-range-picker";
import { ApplicationIngest } from "./application-ingest";
import { ApplicationObservationPanel } from "./application-observation-panel";
import { ServicePicker } from "./pickers";
import {
  ApplicationTraceDetail,
  type ApplicationTraceSelection,
} from "./application-trace-detail";

const tabs = ["overview", "services", "traces", "logs", "ingest"] as const;
type Tab = (typeof tabs)[number];
type SelectedService = Pick<
  MonitoringService,
  "id" | "service_name" | "service_namespace"
>;
type Drill = {
  sourceId?: string;
  traceId?: string;
  operation?: string;
  approximate?: boolean;
};
const defaultTime: DashboardSchemas["DashboardTimeRange"] = {
  kind: "relative",
  seconds: 3600,
};

export function ApplicationObservability({
  application,
  services,
}: {
  application: MonitoringApplication;
  services: (select: (value: SelectedService) => void) => ReactNode;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [tab, setTab] = useState<Tab>("overview");
  const [time, setTime] = useState(defaultTime);
  const [range, setRange] = useState(() => queryScopeBounds(defaultTime));
  const [service, setService] = useState<SelectedService>();
  const [choosing, setChoosing] = useState(false);
  const [drill, setDrill] = useState<Drill>({});
  const [trace, setTrace] = useState<ApplicationTraceSelection>();
  const [tracePage, setTracePage] = useState(1);

  const setTraceQuery = (value: Drill) => {
    setDrill(value);
    setTracePage(1);
  };
  const chooseService = (value?: SelectedService) => {
    setService(value);
    setTraceQuery({});
    setChoosing(false);
  };
  const applyTime = (value: typeof time) => {
    setTime(value);
    setRange(queryScopeBounds(value));
    setTracePage(1);
  };
  const refresh = () => {
    if (time.kind === "relative") applyTime(time);
    else
      void client.invalidateQueries({
        queryKey: ["application-observability", application.id],
      });
  };
  const openServiceTraces = (row: Record<string, unknown>) => {
    if (
      row.resourceId !== application.id ||
      typeof row.serviceId !== "string" ||
      !row.serviceId
    )
      return;
    setService({
      id: row.serviceId,
      service_name: String(row.serviceName ?? ""),
      service_namespace: String(row.serviceNamespace ?? ""),
    });
    setTraceQuery({
      sourceId:
        Array.isArray(row.sourceIds) && row.sourceIds.length > 1
          ? undefined
          : typeof row.sourceId === "string"
            ? row.sourceId
            : undefined,
      operation:
        typeof row.operationName === "string" ? row.operationName : undefined,
    });
    setTab("traces");
  };
  const panelScope = {
    applicationId: application.id,
    range,
    serviceId: service?.id,
  };

  return (
    <section aria-label={t("applicationObservability.title")}>
      <Tabs value={tab} onValueChange={(value) => setTab(value as Tab)}>
        <TabsList>
          {tabs.map((value) => (
            <TabsTrigger key={value} value={value}>
              {t(`applicationObservability.tabs.${value}`)}
            </TabsTrigger>
          ))}
        </TabsList>
        {tab !== "ingest" && (
          <div className="argus-monitoring-toolbar argus-application-filters">
            <TimeRangePicker
              value={time}
              onChange={applyTime}
              label={t("applicationObservability.range")}
            />
            <Button onPress={() => setChoosing(true)}>
              {service
                ? [service.service_namespace, service.service_name]
                    .filter(Boolean)
                    .join(" / ")
                : t("applicationObservability.allServices")}
            </Button>
            {service && (
              <Button onPress={() => chooseService()}>
                {t("applicationObservability.clearService")}
              </Button>
            )}
            <Button onPress={refresh}>
              {t("applicationObservability.refresh")}
            </Button>
          </div>
        )}
        <TabsContent value="overview" className="argus-application-panels">
          <ApplicationObservationPanel
            {...panelScope}
            view="red"
            onSelect={openServiceTraces}
          />
          <ApplicationObservationPanel
            {...panelScope}
            view="topology"
            onSelect={openServiceTraces}
          />
        </TabsContent>
        <TabsContent value="services" className="argus-application-panels">
          {services(chooseService)}
          {service ? (
            <>
              <ApplicationObservationPanel {...panelScope} view="instances" />
              <ApplicationObservationPanel
                {...panelScope}
                view="endpoints"
                onSelect={openServiceTraces}
              />
            </>
          ) : (
            <p className="argus-application-hint">
              {t("applicationObservability.selectServiceDetails")}
            </p>
          )}
        </TabsContent>
        <TabsContent value="traces" className="argus-application-panels">
          <form
            className="argus-monitoring-toolbar"
            onSubmit={(event) => {
              event.preventDefault();
              const fields = new FormData(event.currentTarget);
              setTraceQuery({
                traceId:
                  String(fields.get("traceId") ?? "").trim() || undefined,
                operation:
                  String(fields.get("operation") ?? "").trim() || undefined,
              });
            }}
          >
            <Input
              key={`trace-${drill.traceId ?? ""}`}
              name="traceId"
              defaultValue={drill.traceId}
              aria-label={t("applicationObservability.traceId")}
              placeholder={t("applicationObservability.traceId")}
              maxLength={256}
            />
            <Input
              key={`operation-${drill.operation ?? ""}`}
              name="operation"
              defaultValue={drill.operation}
              aria-label={t("applicationObservability.operation")}
              placeholder={t("applicationObservability.operation")}
              maxLength={4096}
            />
            <Button type="submit">
              {t("applicationObservability.searchTraces")}
            </Button>
            {(drill.sourceId ||
              drill.traceId ||
              drill.operation ||
              drill.approximate) && (
              <Button onPress={() => setTraceQuery({})}>
                {t("applicationObservability.clearDrill")}
              </Button>
            )}
          </form>
          {drill.approximate && (
            <Alert
              title={t("applicationObservability.approximate")}
              description={t("applicationObservability.approximateHint")}
            />
          )}
          <ApplicationObservationPanel
            {...panelScope}
            view="traces"
            drill={drill}
            page={tracePage}
            onPageChange={setTracePage}
            onSelect={(row) => {
              if (
                row.resourceId === application.id &&
                typeof row.traceId === "string" &&
                typeof row.sourceId === "string"
              )
                setTrace({
                  traceId: row.traceId,
                  sourceId: row.sourceId,
                  range,
                });
            }}
          />
        </TabsContent>
        <TabsContent value="logs">
          <ApplicationObservationPanel
            {...panelScope}
            view="logs"
            onSelect={(row) => {
              const traceId =
                typeof row.trace_id === "string" && row.trace_id
                  ? row.trace_id
                  : undefined;
              setTraceQuery({ traceId, approximate: !traceId });
              setTab("traces");
            }}
          />
        </TabsContent>
        <TabsContent value="ingest">
          <ApplicationIngest application={application} />
        </TabsContent>
      </Tabs>
      {choosing && (
        <ServicePicker
          application={application}
          onClose={() => setChoosing(false)}
          onSelect={chooseService}
        />
      )}
      {trace && (
        <ApplicationTraceDetail
          applicationId={application.id}
          selected={trace}
          onClose={() => setTrace(undefined)}
        />
      )}
    </section>
  );
}
