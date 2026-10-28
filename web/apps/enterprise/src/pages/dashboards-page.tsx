import { ActionMenu, DataTable } from "@argus/ui";
import { formatDateTime } from "../components/hosts/host-utils";
import "../styles/resource-lists.css";
import { useDefinitionForm } from "../components/dashboards/use-definition-form";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { QueryBoundary } from "@argus/ui";
import {
  emptyDashboardSpec,
  formatApiError,
  useApi,
  type PendingActionPublic,
  type DashboardItem,
} from "@argus/api-client";
import {
  Alert,
  Badge,
  Button,
  Field,
  FilterBar,
  FormDialog,
  Input,
  PageShell,
  Select,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
  Textarea,
} from "@argus/ui";
import { usePermission } from "../lib/permissions";
import { DashboardActionDialog } from "../components/dashboards/action-dialog";
import "../styles/dashboards.css";

export function DashboardsPage() {
  const api = useApi(),
    { t } = useTranslation(),
    navigate = useNavigate(),
    cache = useQueryClient();
  const canReadResources = usePermission("telemetry.dashboard.read"),
    canManage = usePermission("telemetry.dashboard.manage");
  const canReadModels = usePermission("model.usage.read");
  const canRead = canReadResources || canReadModels;
  const items = useQuery({
    queryKey: ["dashboards"],
    queryFn: () => api.dashboards.list(),
    enabled: canRead,
  });
  const drafts = useQuery({
    queryKey: ["dashboard-drafts"],
    queryFn: () => api.dashboards.drafts(),
    enabled: canManage,
  });
  const folders = useQuery({
    queryKey: ["dashboard-folders"],
    queryFn: () => api.dashboards.folders(),
    enabled: canReadResources,
  });
  const [tab, setTab] = useState("published"),
    [folder, setFolder] = useState("all"),
    [search, setSearch] = useState(""),
    [creating, setCreating] = useState<"dashboard" | "folder" | null>(null),
    [error, setError] = useState(""),
    [action, setAction] = useState<PendingActionPublic | null>(null),
    [busy, setBusy] = useState(false);
  const [name, setName] = useState(""),
    [description, setDescription] = useState(""),
    [newFolder, setNewFolder] = useState("");
  const refresh = async () => {
    setAction(null);
    await Promise.all([
      cache.invalidateQueries({ queryKey: ["dashboards"] }),
      cache.invalidateQueries({ queryKey: ["dashboard-drafts"] }),
      cache.invalidateQueries({ queryKey: ["dashboard-folders"] }),
    ]);
  };
  const form = useDefinitionForm(
    { name, description },
    (v) =>
      Boolean(v.name.trim()) &&
      v.name.length <= 180 &&
      v.description.length <= 2048,
    t("dashboards.editor.invalidForm"),
  );
  const failure = (e: unknown) =>
    setError(
      formatApiError(e, t("dashboards.failed"), (requestId) =>
        t("common.requestReference", { requestId }),
      ),
    );
  const create = async () => {
    setBusy(true);
    setError("");
    try {
      if (creating === "folder") {
        setAction(
          await api.dashboards.previewFolder({
            operation: "folder.create",
            expected_version: 0,
            name,
            description,
            sort_order: 0,
          }),
        );
        setCreating(null);
      } else {
        const draft = await api.dashboards.createDraft({
          name,
          description,
          folder_id: newFolder || undefined,
          spec: emptyDashboardSpec(),
          proposed_bindings: [],
        });
        await navigate({
          to: "/dashboard-drafts/$draftId",
          params: { draftId: draft.id },
        });
      }
    } catch (e) {
      failure(e);
    } finally {
      setBusy(false);
    }
  };
  const lifecycle = async (item: DashboardItem) => {
    try {
      setAction(
        await api.dashboards.previewLifecycle(item.id, {
          operation: item.lifecycle === "active" ? "archive" : "restore",
          expected_version: item.version,
          name: item.name,
          description: item.description,
          sort_order: 0,
        }),
      );
    } catch (e) {
      failure(e);
    }
  };
  if (!canRead)
    return (
      <PageShell title={t("dashboards.title")}>
        <p role="alert">{t("dashboards.forbidden")}</p>
      </PageShell>
    );
  const tabs = ["published", ...(canManage ? ["drafts"] : []), "archived"];
  const searchName = search.trim().toLocaleLowerCase();
  const hasFilters = folder !== "all" || Boolean(searchName);
  const matchesFilters = (item: Pick<DashboardItem, "name" | "folder_id">) =>
    (folder === "all" || folder === (item.folder_id ?? "")) &&
    item.name.toLocaleLowerCase().includes(searchName);
  const visibleDrafts = (drafts.data ?? []).filter(matchesFilters);
  const visible = (items.data ?? []).map((item) => item.managed_template_key === "argus.model-governance" ? { ...item, name: t("modelGovernance.title"), description: t("modelGovernance.managedHint") } : item).filter(
    (item) =>
      (tab === "archived"
        ? item.lifecycle === "archived"
        : item.lifecycle === "active") && matchesFilters(item),
  );
  return (
    <PageShell
      title={t("dashboards.title")}
      description={t("dashboards.description")}
      actions={
        canManage && (
          <div className="argus-dashboard-inline">
            <Button
              onPress={() => {
                setName("");
                setDescription("");
                setCreating("folder");
              }}
            >
              {t("dashboards.createFolder")}
            </Button>
            <Button
              variant="primary"
              onPress={() => {
                setName("");
                setDescription("");
                setCreating("dashboard");
              }}
            >
              {t("dashboards.create")}
            </Button>
          </div>
        )
      }
    >
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          {tabs.map((key) => (
            <TabsTrigger key={key} value={key}>
              {t(`dashboards.${key}`)}
            </TabsTrigger>
          ))}
        </TabsList>
        {tabs.map((key) => (
          <TabsContent
            key={key}
            value={key}
            className="argus-dashboard-directory-panel"
          >
            {tab === key && (
              <>
                <FilterBar
                  className="argus-dashboard-directory-filters"
                  search={{
                    value: search,
                    onChange: setSearch,
                    placeholder: t("dashboards.searchName"),
                  }}
                  filters={[
                    {
                      key: "folder",
                      ariaLabel: t("dashboards.folder"),
                      value: folder,
                      onChange: setFolder,
                      options: [
                        { value: "all", label: t("dashboards.allFolders") },
                        { value: "", label: t("dashboards.ungrouped") },
                        ...(folders.data ?? []).map((f) => ({
                          value: f.id,
                          label: f.name,
                        })),
                      ],
                    },
                  ]}
                />
                {(error || items.isError || drafts.isError) && (
                  <Alert
                    tone="danger"
                    title={t("dashboards.failed")}
                    description={error || t("dashboards.failed")}
                  />
                )}
                {folder !== "all" && folder !== "" && canManage && (
                  <div className="argus-dashboard-inline">
                    {(() => {
                      const f = folders.data?.find((f) => f.id === folder);
                      return (
                        f && (
                          <Button
                            onPress={() =>
                              void api.dashboards
                                .previewFolder({
                                  id: f.id,
                                  operation:
                                    f.status === "active"
                                      ? "folder.archive"
                                      : "folder.restore",
                                  expected_version: f.version,
                                  name: f.name,
                                  description: f.description,
                                  sort_order: f.sort_order,
                                })
                                .then(setAction)
                                .catch(failure)
                            }
                          >
                            {t(
                              f.status === "active"
                                ? "dashboards.archiveFolder"
                                : "dashboards.restoreFolder",
                            )}
                          </Button>
                        )
                      );
                    })()}
                  </div>
                )}
                <QueryBoundary query={tab === "drafts" ? drafts : items}>
                  {tab === "drafts" ? (
                    <DataTable
                      data={visibleDrafts}
                      getRowKey={(d) => d.id}
                      columns={[
                        {
                          key: "name",
                          header: t("resourceList.name"),
                          render: (d) => (
                            <div className="argus-resource-list__identity">
                              <Link
                                to="/dashboard-drafts/$draftId"
                                params={{ draftId: d.id }}
                              >
                                {d.name}
                              </Link>
                              <span className="argus-muted">
                                {d.description}
                              </span>
                            </div>
                          ),
                        },
                        {
                          key: "status",
                          header: t("resourceList.status"),
                          render: () => <Badge>{t("dashboards.draft")}</Badge>,
                        },
                        {
                          key: "folder_id",
                          header: t("resourceList.folder"),
                          render: (d) =>
                            folders.isSuccess
                              ? (folders.data.find((f) => f.id === d.folder_id)
                                  ?.name ?? t("dashboards.ungrouped"))
                              : t("resourceList.unavailable"),
                        },
                        {
                          key: "updated_at",
                          header: t("resourceList.updated"),
                          render: (d) => formatDateTime(d.updated_at),
                        },
                        {
                          key: "actions",
                          header: t("resourceList.actions"),
                          align: "right",
                          render: (d) => (
                            <ActionMenu
                              items={[
                                {
                                  label: t("dashboards.resume"),
                                  onSelect: () =>
                                    void navigate({
                                      to: "/dashboard-drafts/$draftId",
                                      params: { draftId: d.id },
                                    }),
                                },
                              ]}
                            />
                          ),
                        },
                      ]}
                    />
                  ) : (
                    <DataTable
                      data={visible}
                      getRowKey={(d) => d.id}
                      columns={[
                        {
                          key: "name",
                          header: t("resourceList.name"),
                          render: (d) => (
                            <div className="argus-resource-list__identity">
                              <Link
                                to="/dashboards/$dashboardId"
                                params={{ dashboardId: d.id }}
                              >
                                {d.managed_template_key === "argus.model-governance" ? t("modelGovernance.title") : d.name}
                              </Link>
                              <span className="argus-muted">
                                {d.description}
                              </span>
                            </div>
                          ),
                        },
                        {
                          key: "lifecycle",
                          header: t("resourceList.status"),
                          render: (d) => (
                            <Badge>
                              {t(
                                d.managed_template_key
                                  ? "managedDashboards.system"
                                  : d.lifecycle === "active"
                                    ? "dashboards.published"
                                    : "dashboards.archived",
                              )}
                            </Badge>
                          ),
                        },
                        {
                          key: "folder_id",
                          header: t("resourceList.folder"),
                          render: (d) =>
                            folders.isSuccess
                              ? (folders.data.find((f) => f.id === d.folder_id)
                                  ?.name ?? t("dashboards.ungrouped"))
                              : t("resourceList.unavailable"),
                        },
                        {
                          key: "updated_at",
                          header: t("resourceList.updated"),
                          render: (d) => formatDateTime(d.updated_at),
                        },
                        {
                          key: "actions",
                          header: t("resourceList.actions"),
                          align: "right",
                          render: (d) => (
                            <ActionMenu
                              items={[
                                {
                                  label: t("dashboards.open"),
                                  onSelect: () =>
                                    void navigate({
                                      to: "/dashboards/$dashboardId",
                                      params: { dashboardId: d.id },
                                    }),
                                },
                                ...(canManage && !d.managed_template_key
                                  ? [
                                      {
                                        label: t(
                                          d.lifecycle === "active"
                                            ? "dashboards.archive"
                                            : "dashboards.restore",
                                        ),
                                        onSelect: () => void lifecycle(d),
                                      },
                                    ]
                                  : []),
                              ]}
                            />
                          ),
                        },
                      ]}
                    />
                  )}
                  {(tab === "drafts"
                    ? !visibleDrafts.length
                    : !visible.length) &&
                    !items.isLoading && (
                      <div className="argus-dashboard-empty">
                        <h2>
                          {t(
                            hasFilters
                              ? "dashboards.noMatches"
                              : "dashboards.empty",
                          )}
                        </h2>
                        <p>
                          {t(
                            hasFilters
                              ? "dashboards.noMatchesHint"
                              : "dashboards.emptyHint",
                          )}
                        </p>
                      </div>
                    )}
                </QueryBoundary>
              </>
            )}
          </TabsContent>
        ))}
      </Tabs>
      {creating && (
        <FormDialog
          open
          onOpenChange={(open) => !open && setCreating(null)}
          title={t(
            creating === "folder"
              ? "dashboards.createFolder"
              : "dashboards.create",
          )}
          submitLabel={t("dashboards.done")}
          loading={busy}
          onSubmit={form.handleSubmit(() => void create())}
        >
          <div className="argus-dashboard-form-stack">
            <Field label={t("dashboards.name")} requirement="required">
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                maxLength={120}
              />
            </Field>
            <Field
              label={t("dashboards.descriptionField")}
              requirement="optional"
            >
              <Textarea
                value={description}
                onChange={(e) => setDescription(e.target.value)}
              />
            </Field>
            {creating === "dashboard" && (
              <Field label={t("dashboards.folder")} requirement="optional">
                <Select
                  value={newFolder}
                  onValueChange={setNewFolder}
                  options={[
                    { value: "", label: t("dashboards.ungrouped") },
                    ...(folders.data ?? [])
                      .filter((f) => f.status === "active")
                      .map((f) => ({ value: f.id, label: f.name })),
                  ]}
                />
              </Field>
            )}
            {error && <p role="alert">{error}</p>}
          </div>
        </FormDialog>
      )}
      {action && (
        <DashboardActionDialog
          action={action}
          onClose={() => setAction(null)}
          onDone={() => void refresh()}
        />
      )}
    </PageShell>
  );
}
