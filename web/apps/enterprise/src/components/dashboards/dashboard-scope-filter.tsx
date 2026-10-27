import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  formatApiError,
  type DashboardResourceType,
  type DashboardSchemas,
} from "@argus/api-client";
import {
  Button,
  Field,
  ResourcePickerDialog,
  Tabs,
  TabsList,
  TabsTrigger,
} from "@argus/ui";

type Mode = "all_authorized" | "explicit";
type Row = DashboardSchemas["DashboardScopeDirectoryItem"];
const kinds: DashboardResourceType[] = [
  "host",
  "kubernetes_cluster",
  "kubernetes_namespace",
  "application",
  "monitoring_target",
];

export function DashboardScopeFilter({
  dimension,
  mode,
  ids = [],
  initialKind,
  disabled,
  onChange,
}: {
  dimension: "resources" | "applications";
  mode?: Mode;
  ids?: string[];
  initialKind?: DashboardResourceType;
  disabled?: boolean;
  onChange: (mode: Mode | undefined, ids: string[]) => void;
}) {
  const { t } = useTranslation(),
    [open, setOpen] = useState(false);
  const title = t(
    dimension === "resources"
      ? "dashboards.chooseResources"
      : "dashboardControls.chooseApplications",
  );
  const summary =
    mode === "explicit"
      ? t(
          dimension === "resources"
            ? "dashboardControls.resourceCount"
            : "dashboardControls.applicationCount",
          { count: ids.length },
        )
      : t(
          dimension === "resources"
            ? "dashboardControls.allAuthorized"
            : mode === "all_authorized"
              ? "dashboardControls.allApplications"
              : "dashboardControls.unrestrictedApplication",
        );
  return (
    <>
      <Field
        label={t(
          dimension === "resources"
            ? "dashboards.resources"
            : "dashboardControls.applications",
        )}
        requirement="optional"
      >
        <Button
          type="button"
          isDisabled={disabled}
          onPress={() => setOpen(true)}
          aria-label={`${title}: ${summary}`}
        >
          {summary}
        </Button>
      </Field>
      {open && (
        <ScopePicker
          dimension={dimension}
          mode={mode}
          ids={ids}
          initialKind={initialKind}
          title={title}
          onClose={() => setOpen(false)}
          onChange={(next, values) => {
            onChange(next, values);
            setOpen(false);
          }}
        />
      )}
    </>
  );
}

function ScopePicker({
  dimension,
  mode,
  ids,
  initialKind,
  title,
  onClose,
  onChange,
}: {
  dimension: "resources" | "applications";
  mode?: Mode;
  ids: string[];
  initialKind?: DashboardResourceType;
  title: string;
  onClose: () => void;
  onChange: (mode: Mode | undefined, ids: string[]) => void;
}) {
  const api = useApi(),
    { t } = useTranslation();
  const [kind, setKind] = useState<DashboardResourceType>(
    dimension === "applications" ? "application" : (initialKind ?? "host"),
  );
  const [search, setSearch] = useState(""),
    [cursors, setCursors] = useState<Array<string | undefined>>([undefined]);
  const [draftMode, setDraftMode] = useState(mode),
    [draft, setDraft] = useState(ids);
  const query = useQuery({
    queryKey: ["dashboard-scope-directory", kind, search, cursors.at(-1)],
    queryFn: ({ signal }) =>
      api.dashboards.scopeResources(
        { kind, search, cursor: cursors.at(-1), limit: 30 },
        signal,
      ),
  });
  const error = query.error
    ? formatApiError(query.error, t("dashboards.failed"), (requestId) =>
        t("common.requestReference", { requestId }),
      )
    : undefined;
  const summary =
    draftMode === "explicit"
      ? t("dashboardControls.explicitCount", { count: draft.length })
      : t(
          dimension === "resources"
            ? "dashboardControls.allAuthorized"
            : draftMode === "all_authorized"
              ? "dashboardControls.allApplications"
              : "dashboardControls.unrestrictedApplication",
        );
  return (
    <ResourcePickerDialog<Row>
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={title}
      description={
        dimension === "resources"
          ? t("dashboardControls.resourceHint")
          : t("dashboardControls.applicationHint")
      }
      data={query.error ? [] : (query.data?.items ?? [])}
      columns={[
        { key: "name", header: t("dashboardControls.name") },
        {
          key: "environment",
          header: t("monitoring.environment"),
          render: (row) =>
            row.environment
              ? t(`monitoring.env.${row.environment}`, {
                  defaultValue: row.environment,
                })
              : "—",
        },
        ...(dimension === "resources"
          ? [
              {
                key: "parent_name",
                header: t("dashboardControls.parentResource"),
              },
            ]
          : []),
      ]}
      getRowKey={(row) => row.id}
      getRowLabel={(row) => row.name}
      search={search}
      onSearch={(value) => {
        setSearch(value);
        setCursors([undefined]);
      }}
      page={cursors.length}
      loading={query.isFetching}
      error={error}
      hasPrevious={cursors.length > 1}
      hasNext={!!query.data?.next_cursor}
      onPrevious={() => setCursors((v) => v.slice(0, -1))}
      onNext={() => setCursors((v) => [...v, query.data?.next_cursor])}
      selectedKeys={draftMode === "explicit" ? draft : []}
      onToggle={(row) => {
        setDraftMode("explicit");
        setDraft((values) =>
          draftMode !== "explicit"
            ? [row.id]
            : values.includes(row.id)
              ? values.filter((id) => id !== row.id)
              : [...values, row.id],
        );
      }}
      onConfirm={() =>
        onChange(draftMode, draftMode === "explicit" ? draft : [])
      }
      filters={
        <div className="argus-dashboard-scope-picker-options">
          {dimension === "resources" && (
            <Tabs
              value={kind}
              onValueChange={(value) => {
                setKind(value as DashboardResourceType);
                setSearch("");
                setCursors([undefined]);
              }}
            >
              <TabsList>
                {kinds.map((value) => (
                  <TabsTrigger key={value} value={value}>
                    {t(`dashboardControls.resourceKinds.${value}`)}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
          )}
          <div className="argus-dashboard-scope-picker-actions">
            <Button
              type="button"
              onPress={() => {
                setDraftMode(
                  dimension === "resources" ? "all_authorized" : undefined,
                );
                setDraft([]);
              }}
            >
              {t(
                dimension === "resources"
                  ? "dashboardControls.allAuthorized"
                  : "dashboardControls.unrestrictedApplication",
              )}
            </Button>
            {dimension === "applications" && (
              <Button
                type="button"
                onPress={() => {
                  setDraftMode("all_authorized");
                  setDraft([]);
                }}
              >
                {t("dashboardControls.allApplications")}
              </Button>
            )}
            <Button
              type="button"
              onPress={() => {
                setDraftMode("explicit");
                setDraft([]);
              }}
            >
              {t("dashboardControls.clearSelection")}
            </Button>
            <span role="status">{summary}</span>
          </div>
        </div>
      }
    />
  );
}
