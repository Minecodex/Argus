import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useApi, type DashboardSchemas } from "@argus/api-client";
import { Badge, Button, Dialog, KeyValueGrid, Pagination } from "@argus/ui";

type Source = DashboardSchemas["DashboardResolvedSource"];
type SourceResource = {
  id: string;
  type: Source["resource_type"];
  sources: Source[];
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
export function SourceSummary({
  title,
  sources,
}: {
  title: string;
  sources: Source[];
}) {
  const [open, setOpen] = useState(false);
  const { t } = useTranslation();
  const resources = new Map<string, SourceResource>();
  for (const source of sources) {
    const key = `${source.resource_type ?? ""}:${source.resource_id}`;
    const group = resources.get(key);
    if (group) group.sources.push(source);
    else
      resources.set(key, {
        id: source.resource_id,
        type: source.resource_type,
        sources: [source],
      });
  }
  return (
    <>
      <Button
        size="sm"
        variant="ghost"
        aria-label={`${title} ${t("dashboards.resolvedSources")}`}
        onPress={() => setOpen(true)}
      >
        {t("dashboards.resolvedSources")} · {resources.size}
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={`${title} · ${t("dashboards.resolvedSources")}`}
      >
        {open && <SourceResourceList resources={[...resources.values()]} />}
      </Dialog>
    </>
  );
}

function SourceResourceList({ resources }: { resources: SourceResource[] }) {
  const { t } = useTranslation();
  const [page, setPage] = useState(1);
  if (!resources.length) return <p>{t("dashboards.noResolvedSources")}</p>;
  const totalPages = Math.ceil(resources.length / pageSize);
  const current = Math.min(page, totalPages);
  return (
    <div className="argus-dashboard-form-stack">
      <ul className="argus-dashboard-source-resources">
        {resources
          .slice((current - 1) * pageSize, current * pageSize)
          .map((resource) => (
            <SourceResourceRow
              key={`${resource.type ?? ""}:${resource.id}`}
              resource={resource}
            />
          ))}
      </ul>
      {totalPages > 1 && (
        <Pagination page={current} totalPages={totalPages} onChange={setPage} />
      )}
    </div>
  );
}

function SourceResourceRow({ resource }: { resource: SourceResource }) {
  const api = useApi();
  const { t, i18n } = useTranslation();
  const { id, type, sources } = resource;
  const query = useQuery({
    queryKey: ["dashboard-bindings", type, id],
    queryFn: () => api.dashboards.resourceBindings(type!, id),
    enabled: Boolean(type && id),
    retry: false,
  });
  const groups = new Map<string, Source[]>();
  for (const source of sources) {
    const versions = groups.get(source.id);
    if (versions) versions.push(source);
    else groups.set(source.id, [source]);
  }
  return (
    <li className="argus-dashboard-source-resource">
      <div className="argus-dashboard-inline">
        {query.isSuccess && type ? (
          <SourceResourceLink
            id={id}
            type={type}
            name={query.data.resource_name}
          />
        ) : (
          <span>
            {query.isFetching
              ? t("common.loading")
              : t("dashboards.sourceResourceUnavailable")}
          </span>
        )}
        {type && <Badge>{t(`dashboardControls.resourceKinds.${type}`)}</Badge>}
        {[...new Set(sources.map((source) => source.type))].map(
          (sourceType) => (
            <Badge key={sourceType}>{sourceType}</Badge>
          ),
        )}
      </div>
      <details className="argus-dashboard-source-details">
        <summary>{t("dashboards.sourceTechnicalDetails")}</summary>
        <div className="argus-dashboard-form-stack">
          <p className="argus-dashboard-muted">
            {t("dashboards.sourceSnapshotHint")}
          </p>
          {[...groups.entries()].map(([sourceId, versions]) => {
            const source = versions[0]!;
            const registered = versions
              .map((v) => v.registered_at)
              .filter((v): v is string => Boolean(v))
              .sort()[0];
            const date = registered ? new Date(registered) : undefined;
            return (
              <section key={sourceId}>
                <Badge>{installationLabel(source, t)}</Badge>
                <KeyValueGrid
                  columns={2}
                  items={[
                    { label: t("dashboards.sourceIdentity"), value: sourceId },
                    { label: t("dashboards.source"), value: source.type },
                    {
                      label: t("dashboards.resources"),
                      value: id,
                    },
                    {
                      label: t("dashboards.sourceGeneration"),
                      value: source.generation,
                    },
                    {
                      label: t("dashboards.sourceRevisions"),
                      value: [...new Set(versions.map((v) => v.revision))]
                        .sort((a, b) => a - b)
                        .join(", "),
                    },
                    {
                      label: t("dashboards.sourceRegistered"),
                      value:
                        date && Number.isFinite(date.getTime())
                          ? new Intl.DateTimeFormat(i18n.language, {
                              dateStyle: "medium",
                              timeStyle: "medium",
                            }).format(date)
                          : "—",
                    },
                  ]}
                />
              </section>
            );
          })}
        </div>
      </details>
    </li>
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
