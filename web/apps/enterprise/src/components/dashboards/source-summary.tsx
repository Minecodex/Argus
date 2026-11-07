import { useState } from "react";
import { skipToken, useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useApi, type DashboardSchemas } from "@argus/api-client";
import {
  Badge,
  Button,
  DataTable,
  Dialog,
  Pagination,
  type Column,
} from "@argus/ui";

type Source = DashboardSchemas["DashboardResolvedSource"];
type SourceRow = {
  key: string;
  source: Source;
  revisions: Set<number>;
  registeredAt: number;
};
const pageSize = 20;
export function installationLabel(source: Source, t: (key: string) => string) {
  return t(
    source.current_installation === undefined
      ? "dashboards.unknownInstallation"
      : source.current_installation
        ? "dashboards.currentInstallation"
        : "dashboards.historicalInstallation",
  );
}
export function sourceValueLabels(
  sources: Source[],
  t: (key: string) => string,
): Record<string, string> {
  return Object.fromEntries(
    sources.map((source) => [
      source.id,
      `${installationLabel(source, t)} · ${source.id}`,
    ]),
  );
}

export function sourceRows(sources: Source[]) {
  const rows = new Map<string, SourceRow>();
  for (const source of sources) {
    const key = `${source.resource_type ?? ""}:${source.resource_id}:${source.id}:${source.generation}`;
    const row = rows.get(key) ?? {
      key,
      source,
      revisions: new Set<number>(),
      registeredAt: Infinity,
    };
    row.revisions.add(source.revision);
    if (source.revision > row.source.revision) row.source = source;
    const timestamp = Date.parse(source.registered_at ?? "");
    if (Number.isFinite(timestamp))
      row.registeredAt = Math.min(row.registeredAt, timestamp);
    rows.set(key, row);
  }
  return [...rows.values()];
}

export function SourceSummary({
  title,
  sources,
}: {
  title: string;
  sources: Source[];
}) {
  const [open, setOpen] = useState(false);
  const { t } = useTranslation();
  const rows = sourceRows(sources);
  return (
    <>
      <Button
        size="sm"
        variant="ghost"
        aria-label={`${title} ${t("dashboards.resolvedSources")}`}
        onPress={() => setOpen(true)}
      >
        {t("dashboards.resolvedSources")} · {rows.length}
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={`${title} · ${t("dashboards.resolvedSources")}`}
        description={
          rows.length ? t("dashboards.sourceSnapshotHint") : undefined
        }
        size="lg"
      >
        {open && <SourceTable rows={rows} />}
      </Dialog>
    </>
  );
}

function SourceTable({ rows }: { rows: SourceRow[] }) {
  const { t, i18n } = useTranslation();
  const [page, setPage] = useState(1);
  if (!rows.length) return <p>{t("dashboards.noResolvedSources")}</p>;
  const totalPages = Math.ceil(rows.length / pageSize);
  const current = Math.min(page, totalPages);
  const dateFormat = new Intl.DateTimeFormat(i18n.language, {
    dateStyle: "medium",
    timeStyle: "medium",
  });
  const columns: Column<SourceRow>[] = [
    {
      key: "resource",
      header: t("dashboards.sourceResourceName"),
      cellClassName: "argus-dashboard-source-text",
      render: ({ source }) => <SourceResourceName source={source} />,
    },
    {
      key: "resourceType",
      header: t("dashboardControls.resourceType"),
      render: ({ source }) =>
        source.resource_type
          ? t(`dashboardControls.resourceKinds.${source.resource_type}`)
          : "—",
    },
    {
      key: "name",
      header: t("dashboards.sourceName"),
      cellClassName: "argus-dashboard-source-text",
      render: ({ source }) => (
        <span className="argus-dashboard-inline">
          {source.name && <span>{source.name}</span>}
          <Badge>{source.type}</Badge>
        </span>
      ),
    },
    {
      key: "installation",
      header: t("dashboards.sourceInstallation"),
      render: ({ source }) => <Badge>{installationLabel(source, t)}</Badge>,
    },
    {
      key: "revisions",
      header: t("dashboards.sourceRevisions"),
      cellClassName: "argus-dashboard-source-text",
      render: ({ revisions }) =>
        [...revisions].sort((a, b) => a - b).join(", "),
    },
    {
      key: "registeredAt",
      header: t("dashboards.sourceRegistered"),
      render: ({ registeredAt }) =>
        Number.isFinite(registeredAt) ? dateFormat.format(registeredAt) : "—",
    },
  ];
  return (
    <div className="argus-dashboard-form-stack">
      <DataTable
        columns={columns}
        data={rows.slice((current - 1) * pageSize, current * pageSize)}
        getRowKey={(row) => row.key}
        density="compact"
        tableClassName="argus-dashboard-source-table"
      />
      {totalPages > 1 && (
        <Pagination page={current} totalPages={totalPages} onChange={setPage} />
      )}
    </div>
  );
}

function SourceResourceName({ source }: { source: Source }) {
  const api = useApi();
  const { t } = useTranslation();
  const { resource_id: id, resource_type: type } = source;
  const query = useQuery({
    queryKey: ["dashboard-bindings", type, id],
    queryFn:
      type && id ? () => api.dashboards.resourceBindings(type, id) : skipToken,
    staleTime: 0,
    retry: false,
  });
  if (query.isFetching) return t("common.loading");
  return query.isSuccess && query.data.resource_name && type ? (
    <SourceResourceLink id={id} type={type} name={query.data.resource_name} />
  ) : (
    t("dashboards.sourceResourceUnavailable")
  );
}

function SourceResourceLink({
  id,
  type,
  name,
}: {
  id: string;
  type: NonNullable<Source["resource_type"]>;
  name: string;
}) {
  const props = { className: "argus-dashboard-resource-link", children: name };
  switch (type) {
    case "host":
      return <Link to="/hosts/$hostId" params={{ hostId: id }} {...props} />;
    case "kubernetes_cluster":
      return (
        <Link
          to="/kubernetes/$clusterId"
          params={{ clusterId: id }}
          {...props}
        />
      );
    case "application":
      return (
        <Link
          to="/applications/$applicationId"
          params={{ applicationId: id }}
          {...props}
        />
      );
    case "monitoring_target":
      return <Link to="/monitoring" search={{ target: id }} {...props} />;
  }
}
