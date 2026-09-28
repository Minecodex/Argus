import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
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
  FormDrawer,
  Input,
  PageShell,
  Select,
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
  const canRead = usePermission("telemetry.dashboard.read"),
    canManage = usePermission("telemetry.dashboard.manage");
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
    enabled: canRead,
  });
  const [tab, setTab] = useState("published"),
    [folder, setFolder] = useState("all"),
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
  const visible = (items.data ?? []).filter(
    (item) =>
      (tab === "archived"
        ? item.lifecycle === "archived"
        : item.lifecycle === "active") &&
      (folder === "all" || folder === (item.folder_id ?? "")),
  );
  return (
    <PageShell
      title={t("dashboards.title")}
      description={t("dashboards.description")}
      actions={
        canManage && (
          <div className="argus-dashboard-inline">
            <Button
              onClick={() => {
                setName("");
                setDescription("");
                setCreating("folder");
              }}
            >
              {t("dashboards.createFolder")}
            </Button>
            <Button
              variant="primary"
              onClick={() => {
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
      <div className="argus-dashboard-toolbar">
        <div role="tablist" aria-label={t("dashboards.title")}>
          {["published", ...(canManage ? ["drafts"] : []), "archived"].map(
            (key) => (
              <Button
                role="tab"
                aria-selected={tab === key}
                key={key}
                variant={tab === key ? "primary" : "ghost"}
                onClick={() => setTab(key)}
              >
                {t(`dashboards.${key}`)}
              </Button>
            ),
          )}
        </div>
        <Select
          ariaLabel={t("dashboards.folder")}
          value={folder}
          onValueChange={setFolder}
          options={[
            { value: "all", label: t("dashboards.allFolders") },
            { value: "", label: t("dashboards.ungrouped") },
            ...(folders.data ?? []).map((f) => ({
              value: f.id,
              label: f.name,
            })),
          ]}
        />
      </div>
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
                  onClick={() =>
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
      <div className="argus-dashboard-directory">
        {tab === "drafts"
          ? (drafts.data ?? []).map((d) => (
              <article className="argus-dashboard-directory-card" key={d.id}>
                <Badge>{t("dashboards.draft")}</Badge>
                <h2>{d.name}</h2>
                <p>{d.description}</p>
                <Button
                  onClick={() =>
                    void navigate({
                      to: "/dashboard-drafts/$draftId",
                      params: { draftId: d.id },
                    })
                  }
                >
                  {t("dashboards.resume")}
                </Button>
              </article>
            ))
          : visible.map((item) => (
              <article className="argus-dashboard-directory-card" key={item.id}>
                <div className="argus-dashboard-inline">
                  <Badge>
                    {t(
                      `dashboards.${item.lifecycle === "active" ? "published" : "archived"}`,
                    )}
                  </Badge>
                  <span>
                    {folders.data?.find((f) => f.id === item.folder_id)?.name ??
                      t("dashboards.ungrouped")}
                  </span>
                </div>
                <h2>
                  <button
                    className="argus-dashboard-link"
                    onClick={() =>
                      void navigate({
                        to: "/dashboards/$dashboardId",
                        params: { dashboardId: item.id },
                      })
                    }
                  >
                    {item.name}
                  </button>
                </h2>
                <p>{item.description}</p>
                <div className="argus-dashboard-inline">
                  <Button
                    onClick={() =>
                      void navigate({
                        to: "/dashboards/$dashboardId",
                        params: { dashboardId: item.id },
                      })
                    }
                  >
                    {t("dashboards.open")}
                  </Button>
                  {canManage && (
                    <Button
                      variant="ghost"
                      onClick={() => void lifecycle(item)}
                    >
                      {t(
                        item.lifecycle === "active"
                          ? "dashboards.archive"
                          : "dashboards.restore",
                      )}
                    </Button>
                  )}
                </div>
              </article>
            ))}
      </div>
      {(tab === "drafts" ? !drafts.data?.length : !visible.length) &&
        !items.isLoading && (
          <div className="argus-dashboard-empty">
            <h2>{t("dashboards.empty")}</h2>
            <p>{t("dashboards.emptyHint")}</p>
          </div>
        )}
      {creating && (
        <FormDrawer
          open
          onOpenChange={(open) => !open && setCreating(null)}
          title={t(
            creating === "folder"
              ? "dashboards.createFolder"
              : "dashboards.create",
          )}
          submitLabel={t("dashboards.done")}
          loading={busy}
          onSubmit={() => void create()}
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
        </FormDrawer>
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
