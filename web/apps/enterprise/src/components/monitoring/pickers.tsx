import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  formatApiError,
  monitoringCollectorCompatible,
  useApi,
  type MonitoringApplication,
  type MonitoringService,
  type MonitoringIntegration,
  type MonitoringSchemas,
} from "@argus/api-client";
import { Badge, ResourcePickerDialog, type Column } from "@argus/ui";

export type ApplicationAssociation = {
  application: MonitoringApplication;
  service?: MonitoringService;
};
export type CollectorChoice = MonitoringSchemas["MonitoringCollectorChoice"];

function usePickerPage() {
  const [search, setSearch] = useState(""),
    [cursors, setCursors] = useState<Array<string | undefined>>([undefined]);
  return {
    search,
    cursor: cursors.at(-1),
    page: cursors.length,
    previous: () => setCursors((old) => old.slice(0, -1)),
    next: (cursor?: string) => {
      if (cursor) setCursors((old) => [...old, cursor]);
    },
    searchFor: (value: string) => {
      setSearch(value);
      setCursors([undefined]);
    },
  };
}
function usePickerError() {
  const { t } = useTranslation();
  return (error: unknown) =>
    error
      ? formatApiError(error, t("monitoring.errors.unavailable"), (requestId) =>
          t("common.requestReference", { requestId }),
        )
      : undefined;
}

export function ApplicationPicker({
  selected,
  multiple = true,
  onDone,
  onClose,
}: {
  selected: ApplicationAssociation[];
  multiple?: boolean;
  onDone: (rows: ApplicationAssociation[]) => void;
  onClose: () => void;
}) {
  const api = useApi(),
    { t } = useTranslation(),
    pager = usePickerPage(),
    error = usePickerError();
  const [draft, setDraft] = useState(selected);
  const query = useQuery({
    queryKey: ["monitoring", "application-picker", pager.search, pager.cursor],
    queryFn: () =>
      api.monitoring.applications({
        search: pager.search,
        cursor: pager.cursor,
        limit: 30,
      }),
  });
  const columns: Column<MonitoringApplication>[] = [
    { key: "name", header: t("monitoring.name") },
    {
      key: "environment",
      header: t("monitoring.environment"),
      render: (item) => (
        <Badge>
          {t(`monitoring.env.${item.environment}`, {
            defaultValue: item.environment,
          })}
        </Badge>
      ),
    },
    { key: "owner_name", header: t("monitoring.owner") },
  ];
  return (
    <ResourcePickerDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t("monitoring.chooseApplications")}
      description={t("monitoring.applicationContextHint")}
      data={query.data?.items ?? []}
      columns={columns}
      getRowKey={(item) => item.id}
      getRowLabel={(item) => `${item.name} ${item.environment}`}
      search={pager.search}
      onSearch={pager.searchFor}
      loading={query.isFetching}
      error={error(query.error)}
      page={pager.page}
      hasPrevious={pager.page > 1}
      hasNext={Boolean(query.data?.next_cursor)}
      onPrevious={pager.previous}
      onNext={() => pager.next(query.data?.next_cursor)}
      selectedKeys={draft.map((item) => item.application.id)}
      onSelect={
        multiple ? undefined : (item) => onDone([{ application: item }])
      }
      onToggle={
        multiple
          ? (item) =>
              setDraft((old) =>
                old.some((value) => value.application.id === item.id)
                  ? old.filter((value) => value.application.id !== item.id)
                  : [...old, { application: item }],
              )
          : undefined
      }
      onConfirm={multiple ? () => onDone(draft) : undefined}
    />
  );
}

export function ServicePicker({
  application,
  onSelect,
  onClose,
}: {
  application: MonitoringApplication;
  onSelect: (row: MonitoringService) => void;
  onClose: () => void;
}) {
  const api = useApi(),
    { t } = useTranslation(),
    pager = usePickerPage(),
    error = usePickerError();
  const query = useQuery({
    queryKey: [
      "monitoring",
      "service-picker",
      application.id,
      pager.search,
      pager.cursor,
    ],
    queryFn: () =>
      api.monitoring.services({
        application_ids: [application.id],
        search: pager.search,
        cursor: pager.cursor,
        limit: 30,
      }),
  });
  return (
    <ResourcePickerDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t("monitoring.chooseService")}
      description={`${application.name} · ${t(`monitoring.env.${application.environment}`, { defaultValue: application.environment })}`}
      data={query.data?.items ?? []}
      columns={[
        { key: "service_name", header: t("monitoring.serviceName") },
        { key: "service_namespace", header: t("monitoring.serviceNamespace") },
        { key: "runtime", header: t("monitoring.runtime") },
      ]}
      getRowKey={(item) => item.id}
      getRowLabel={(item) => item.service_name}
      search={pager.search}
      onSearch={pager.searchFor}
      loading={query.isFetching}
      error={error(query.error)}
      page={pager.page}
      hasPrevious={pager.page > 1}
      hasNext={Boolean(query.data?.next_cursor)}
      onPrevious={pager.previous}
      onNext={() => pager.next(query.data?.next_cursor)}
      onSelect={onSelect}
    />
  );
}

export function CollectorPicker({
  integration,
  requiredComponents = [],
  resourceType,
  onSelect,
  onClose,
}: {
  integration: Pick<MonitoringIntegration, "component" | "platforms">;
  requiredComponents?: string[];
  resourceType?: "host" | "kubernetes_cluster";
  onSelect: (row: CollectorChoice) => void;
  onClose: () => void;
}) {
  const api = useApi(),
    { t } = useTranslation(),
    pager = usePickerPage(),
    error = usePickerError();
  const query = useQuery({
    queryKey: ["monitoring", "collector-picker", pager.search, pager.cursor],
    queryFn: () =>
      api.monitoring.collectors({
        search: pager.search,
        cursor: pager.cursor,
        limit: 30,
      }),
  });
  const compatible = (item: CollectorChoice) =>
    monitoringCollectorCompatible(
      item,
      integration,
      requiredComponents,
      resourceType,
    );
  return (
    <ResourcePickerDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t("monitoring.chooseCollector")}
      description={t("monitoring.collectorHint")}
      data={query.data?.items ?? []}
      columns={[
        { key: "name", header: t("monitoring.executionResource") },
        { key: "platform", header: t("monitoring.platform") },
        { key: "collector_version", header: t("monitoring.collectorVersion") },
        {
          key: "status",
          header: t("monitoring.status"),
          render: (item) =>
            t(`monitoring.collectorStates.${item.status}`, {
              defaultValue: item.status,
            }),
        },
        {
          key: "compatible",
          header: t("monitoring.capability"),
          render: (item) =>
            t(
              compatible(item)
                ? "monitoring.compatible"
                : "monitoring.upgradeRequired",
            ),
        },
      ]}
      getRowKey={(item) => item.id}
      getRowLabel={(item) => item.name}
      isDisabled={(item) => !compatible(item)}
      search={pager.search}
      onSearch={pager.searchFor}
      loading={query.isFetching}
      error={error(query.error)}
      page={pager.page}
      hasPrevious={pager.page > 1}
      hasNext={Boolean(query.data?.next_cursor)}
      onPrevious={pager.previous}
      onNext={() => pager.next(query.data?.next_cursor)}
      onSelect={onSelect}
    />
  );
}
