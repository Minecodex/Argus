import { useState } from "react";
import { Link, useParams } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Plus, Search } from "lucide-react";
import {
  formatApiError,
  useApi,
  type MonitoringApplication,
  type MonitoringApplicationInput,
} from "@argus/api-client";
import {
  Badge,
  Button,
  DataTable,
  EmptyState,
  Field,
  FormDialog,
  Input,
  PageShell,
  Select,
  type Column,
} from "@argus/ui";
import { usePermission } from "../lib/permissions";
import { useMonitoringActions } from "../components/monitoring/actions";
import { ApplicationIngest } from "../components/monitoring/application-ingest";
import { ServiceDeclaration } from "../components/monitoring/service-declaration";
import { ApplicationObservability } from "../components/monitoring/application-observability";
import "../styles/monitoring.css";

export function ApplicationsPage() {
  const params = useParams({ strict: false });
  return params.applicationId ? (
    <ApplicationDetails id={params.applicationId} />
  ) : (
    <ApplicationList />
  );
}

function ApplicationList() {
  const api = useApi(),
    { t } = useTranslation();
  const canRead = usePermission("application.read"),
    canManage = usePermission("application.manage");
  const [search, setSearch] = useState(""),
    [cursors, setCursors] = useState<Array<string | undefined>>([undefined]);
  const [creating, setCreating] = useState(false),
    actions = useMonitoringActions();
  const query = useQuery({
    queryKey: ["monitoring", "applications", search, cursors.at(-1)],
    queryFn: () =>
      api.monitoring.applications({
        search,
        cursor: cursors.at(-1),
        limit: 50,
      }),
    enabled: canRead,
  });
  const columns: Column<MonitoringApplication>[] = [
    {
      key: "name",
      header: t("monitoring.name"),
      render: (a) => (
        <Link
          to="/applications/$applicationId"
          params={{ applicationId: a.id }}
        >
          {a.name}
        </Link>
      ),
    },
    {
      key: "environment",
      header: t("monitoring.environment"),
      render: (a) => (
        <Badge>
          {t(`monitoring.env.${a.environment}`, {
            defaultValue: a.environment,
          })}
        </Badge>
      ),
    },
    {
      key: "owner_name",
      header: t("monitoring.owner"),
      render: (a) => a.owner_name || "—",
    },
    {
      key: "status",
      header: t("monitoring.status"),
      render: (a) => t(`monitoring.${a.status}`),
    },
  ];
  return (
    <PageShell
      title={t("monitoring.applications")}
      description={t("monitoring.description")}
      actions={
        canManage && (
          <Button variant="primary" onPress={() => setCreating(true)}>
            <Plus size={16} />
            {t("monitoring.createApplication")}
          </Button>
        )
      }
    >
      {!canRead ? (
        <EmptyState
          title={t("monitoring.denied")}
          description={t("monitoring.errors.denied")}
        />
      ) : (
        <>
          <div className="argus-monitoring-toolbar">
            <Search size={16} />
            <Input
              aria-label={t("monitoring.search")}
              placeholder={t("monitoring.search")}
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                setCursors([undefined]);
              }}
            />
          </div>
          {query.isPending && <p role="status">{t("common.loading")}</p>}
          {query.error && (
            <p role="alert">
              {formatApiError(
                query.error,
                t("monitoring.errors.unavailable"),
                (requestId) => t("common.requestReference", { requestId }),
              )}
            </p>
          )}
          {query.data &&
            (query.data.items.length ? (
              <DataTable
                data={query.data.items}
                columns={columns}
                getRowKey={(a) => a.id}
              />
            ) : (
              <EmptyState
                title={t("monitoring.emptyApplications")}
                description={t("monitoring.emptyApplicationsDescription")}
              />
            ))}
          <div className="argus-monitoring-pagination">
            <Button
              isDisabled={cursors.length === 1 || query.isFetching}
              onPress={() => setCursors((v) => v.slice(0, -1))}
            >
              {t("monitoring.previous")}
            </Button>
            <span>{t("monitoring.page", { page: cursors.length })}</span>
            <Button
              isDisabled={!query.data?.next_cursor || query.isFetching}
              onPress={() => setCursors((v) => [...v, query.data!.next_cursor])}
            >
              {t("monitoring.next")}
            </Button>
          </div>
        </>
      )}
      {actions.error && <p role="alert">{actions.error}</p>}
      {creating && (
        <ApplicationEditor
          open
          onClose={() => setCreating(false)}
          busy={actions.busy}
          onSave={async (application) => {
            if (
              await actions.preview({
                kind: "application",
                operation: "create",
                application,
              })
            )
              setCreating(false);
          }}
        />
      )}
      {actions.confirmation}
    </PageShell>
  );
}

function ApplicationDetails({ id }: { id: string }) {
  const api = useApi(),
    { t, i18n } = useTranslation(),
    canRead = usePermission("application.read"),
    canManage = usePermission("application.manage");
  const [editing, setEditing] = useState(false),
    actions = useMonitoringActions();
  const [declaringService, setDeclaringService] = useState(false);
  const [cursors, setCursors] = useState<Array<string | undefined>>([
    undefined,
  ]);
  const app = useQuery({
    queryKey: ["monitoring", "application", id],
    queryFn: () => api.monitoring.application(id),
    enabled: canRead,
  });
  const services = useQuery({
    queryKey: ["monitoring", "services", id, cursors.at(-1)],
    queryFn: () =>
      api.monitoring.services({
        application_ids: [id],
        limit: 50,
        cursor: cursors.at(-1),
      }),
    enabled: canRead && !!app.data,
  });
  const row = app.data;
  return (
    <PageShell
      title={row?.name ?? t("monitoring.applications")}
      description={row?.description}
      breadcrumbs={[
        { label: t("monitoring.applications"), href: "/applications" },
        { label: row?.name ?? t("common.loading") },
      ]}
      actions={
        row?.status === "active" &&
        canManage && (
          <div className="argus-monitoring-toolbar">
            <Button onPress={() => setEditing(true)}>
              {t("monitoring.editApplication")}
            </Button>
            <Button
              isDisabled={actions.busy}
              onPress={() =>
                void actions.preview({
                  kind: "application",
                  operation: "archive",
                  id,
                  expected_version: row.version,
                })
              }
            >
              {t("monitoring.archiveApplication")}
            </Button>
          </div>
        )
      }
    >
      {!canRead && (
        <EmptyState
          title={t("monitoring.denied")}
          description={t("monitoring.errors.denied")}
        />
      )}
      {app.isPending && canRead && <p role="status">{t("common.loading")}</p>}
      {(app.error || actions.error) && (
        <p role="alert">
          {actions.error ||
            formatApiError(
              app.error,
              t("monitoring.errors.unavailable"),
              (requestId) => t("common.requestReference", { requestId }),
            )}
        </p>
      )}
      {row && (
        <>
          <dl className="argus-monitoring-facts">
            <div>
              <dt>{t("monitoring.environment")}</dt>
              <dd>
                {t(`monitoring.env.${row.environment}`, {
                  defaultValue: row.environment,
                })}
              </dd>
            </div>
            <div>
              <dt>{t("monitoring.owner")}</dt>
              <dd>{row.owner_name || "—"}</dd>
            </div>
            <div>
              <dt>{t("monitoring.status")}</dt>
              <dd>{t(`monitoring.${row.status}`)}</dd>
            </div>
          </dl>
          <h2>{t("monitoring.services")}</h2>
          {canManage && row.status === "active" && (
            <Button onPress={() => setDeclaringService(true)}>
              {t("monitoring.createService")}
            </Button>
          )}
          {services.error && (
            <p role="alert">
              {formatApiError(
                services.error,
                t("monitoring.errors.unavailable"),
                (requestId) => t("common.requestReference", { requestId }),
              )}
            </p>
          )}
          {services.data &&
            (services.data.items.length ? (
              <DataTable
                data={services.data.items}
                getRowKey={(s) => s.id}
                columns={[
                  { key: "service_name", header: t("monitoring.serviceName") },
                  {
                    key: "service_namespace",
                    header: t("monitoring.serviceNamespace"),
                  },
                  { key: "runtime", header: t("monitoring.runtime") },
                  {
                    key: "last_seen_at",
                    header: t("monitoring.lastSeen"),
                    render: (s) =>
                      s.last_seen_at
                        ? new Date(s.last_seen_at).toLocaleString(i18n.language)
                        : t("monitoring.serviceNotObserved"),
                  },
                ]}
              />
            ) : (
              <EmptyState
                title={t("monitoring.noServices")}
                description={t("monitoring.noServicesDescription")}
              />
            ))}
          <div className="argus-monitoring-pagination">
            <Button
              isDisabled={cursors.length === 1}
              onPress={() => setCursors((v) => v.slice(0, -1))}
            >
              {t("monitoring.previous")}
            </Button>
            <Button
              isDisabled={!services.data?.next_cursor}
              onPress={() =>
                setCursors((v) => [...v, services.data!.next_cursor])
              }
            >
              {t("monitoring.next")}
            </Button>
          </div>
          <ApplicationObservability key={row.id} application={row} />
          <ApplicationIngest application={row} />
          {declaringService && (
            <ServiceDeclaration
              applicationId={id}
              busy={actions.busy}
              onClose={() => setDeclaringService(false)}
              onSave={actions.preview}
            />
          )}
          {editing && (
            <ApplicationEditor
              open
              initial={row}
              onClose={() => setEditing(false)}
              busy={actions.busy}
              onSave={async (application) => {
                if (
                  await actions.preview({
                    kind: "application",
                    operation: "update",
                    id,
                    expected_version: row.version,
                    application,
                  })
                )
                  setEditing(false);
              }}
            />
          )}
        </>
      )}
      {actions.confirmation}
    </PageShell>
  );
}

function ApplicationEditor({
  open,
  initial,
  busy,
  onClose,
  onSave,
}: {
  open: boolean;
  initial?: MonitoringApplication;
  busy: boolean;
  onClose: () => void;
  onSave: (input: MonitoringApplicationInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(initial?.name ?? ""),
    [environment, setEnvironment] = useState(
      initial?.environment ?? "production",
    ),
    [owner, setOwner] = useState(initial?.owner_name ?? ""),
    [description, setDescription] = useState(initial?.description ?? "");
  return (
    <FormDialog
      open={open}
      onOpenChange={(value) => {
        if (!value) onClose();
      }}
      title={t(
        initial ? "monitoring.editApplication" : "monitoring.createApplication",
      )}
      submitLabel={t("monitoring.preview")}
      loading={busy}
      onSubmit={() =>
        void onSave({
          name: name.trim(),
          environment,
          owner_name: owner.trim(),
          description,
        })
      }
    >
      <Field label={t("monitoring.name")} requirement="required">
        <Input
          required
          maxLength={200}
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </Field>
      <Field
        label={t("monitoring.environment")}
        requirement="required"
        hint={t("monitoring.immutableEnvironment")}
      >
        <Select
          value={environment}
          onValueChange={setEnvironment}
          disabled={!!initial}
          options={["production", "testing", "staging", "development"].map(
            (value) => ({ value, label: t(`monitoring.env.${value}`) }),
          )}
        />
      </Field>
      <Field label={t("monitoring.owner")} requirement="optional">
        <Input
          maxLength={200}
          value={owner}
          onChange={(e) => setOwner(e.target.value)}
        />
      </Field>
      <Field label={t("monitoring.descriptionField")} requirement="optional">
        <Input
          maxLength={4096}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
      </Field>
    </FormDialog>
  );
}
