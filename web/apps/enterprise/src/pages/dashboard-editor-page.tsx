import { useEffect, useState } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  ApiError,
  formatApiError,
  useApi,
  type DashboardExecution,
  type DashboardRevision,
  type DashboardDraft,
  type DashboardItem,
  type PendingActionPublic,
} from "@argus/api-client";
import {
  Alert,
  Badge,
  Button,
  ConfirmDialog,
  DashboardGrid,
  Dialog,
  Field,
  FormDrawer,
  Input,
  PageShell,
  Select,
  Textarea,
} from "@argus/ui";
import { usePermission } from "../lib/permissions";
import { DashboardActionDialog } from "../components/dashboards/action-dialog";

import { PanelTile } from "../components/dashboards/panel-tile";
import { VariablesEditor } from "../components/dashboards/variables-editor";
import { DashboardSettingsEditor } from "../components/dashboards/dashboard-settings-editor";
import { useDashboardDraftWorkspace } from "../components/dashboards/draft-workspace-context";
import { draftInput } from "../components/dashboards/model";
import { PanelPresetPicker } from "../components/dashboards/panel-preset-picker";
import { useDefinitionForm } from "../components/dashboards/use-definition-form";
import "../styles/dashboards.css";

export function DashboardEditorPage() {
  const { draftId } = useParams({
    from: "/authed/admin/dashboard-drafts/$draftId",
  });
  const api = useApi(),
    { t } = useTranslation(),
    navigate = useNavigate();
  const canManage = usePermission("telemetry.dashboard.manage");
  const editor = useDashboardDraftWorkspace();
  const folders = useQuery({
    queryKey: ["dashboard-folders"],
    queryFn: () => api.dashboards.folders(),
    enabled: canManage,
  });
  const [addingPanel, setAddingPanel] = useState(false),
    [variables, setVariables] = useState(false),
    [metadata, setMetadata] = useState(false),
    [settings, setSettings] = useState(false),
    [sample, setSample] = useState<DashboardExecution>(),
    [sampleStatus, setSampleStatus] = useState("");
  const [sampleVersion, setSampleVersion] = useState<number>();
  const [action, setAction] = useState<PendingActionPublic | null>(null),
    [busy, setBusy] = useState(false),
    [remove, setRemove] = useState<string>(),
    [discard, setDiscard] = useState(false),
    [published, setPublished] = useState<DashboardRevision>(),
    [conflictDraft, setConflictDraft] = useState<DashboardDraft>(),
    [conflictObject, setConflictObject] = useState<DashboardItem>(),
    [message, setMessage] = useState("");
  const draft = editor.draft;
  const handle = (e: unknown) => {
    editor.setError(e);
    if (e instanceof ApiError && e.code === "DASHBOARD_VERSION_CONFLICT") {
      editor.setConflict(true);
      setAction(null);
    }
  };
  const run = async (kind: "sample" | "preview" | "generate", id?: string) => {
    setBusy(true);
    setMessage("");
    try {
      const saved = await editor.save();
      if (kind === "preview")
        setAction(await api.dashboards.preview(draftId, saved.draft_version));
      else if (kind === "sample") {
        const result = await api.dashboards.sample(draftId, {
          expected_version: saved.draft_version,
        });
        setSample(result.sample.execution as DashboardExecution | undefined);
        setSampleVersion(saved.draft_version);
        setSampleStatus(String(result.sample.status ?? "unavailable"));
        if (!result.validation.valid)
          setMessage(
            result.validation.issues
              .map((i) => `${i.path}: ${i.message}`)
              .join("\n"),
          );
      } else {
        let messages: string[] = [];
        await editor.mutate(async (stored) => {
          const generated = await api.dashboards.generateDrilldowns(draftId, {
            expected_version: stored.draft_version,
            panel_id: id!,
            signal_sources: {},
          });
          messages = generated.issues.map((i) => i.message);
          return generated.draft;
        });
        setMessage(messages.join("\n"));
      }
    } catch (e) {
      handle(e);
    } finally {
      setBusy(false);
    }
  };
  useEffect(() => {
    if (!editor.conflict) return;
    let active = true;
    setConflictDraft(undefined);
    void Promise.all([
      api.dashboards.draft(draftId),
      draft?.dashboard_id
        ? api.dashboards.get(draft.dashboard_id)
        : Promise.resolve(undefined),
    ])
      .then(([serverDraft, data]) => {
        if (active) {
          setConflictDraft(serverDraft);
          setPublished(data?.revision);
          setConflictObject(data?.dashboard);
        }
      })
      .catch(editor.setError);
    return () => {
      active = false;
    };
  }, [editor.conflict, draft?.dashboard_id, draftId, api, editor.setError]);
  const resolve = async () => {
    if (!conflictDraft) return;
    setBusy(true);
    try {
      const local = structuredClone(editor.current.current!);
      // Confirm exactly the versions shown in the comparison, never fetch and approve unseen changes.
      const rebased =
        published && conflictObject
          ? await api.dashboards.rebase(draftId, {
              expected_version: conflictDraft.draft_version,
              object_version: conflictObject.version,
              revision_id: published.id,
            })
          : conflictDraft;
      const saved = await api.dashboards.saveDraft(draftId, {
        ...draftInput(local),
        expected_version: rebased.draft_version,
      });
      editor.install(saved);
    } catch (e) {
      handle(e);
    } finally {
      setBusy(false);
    }
  };
  if (!canManage)
    return (
      <PageShell title={t("dashboards.title")}>
        <p role="alert">{t("dashboards.noEdit")}</p>
      </PageShell>
    );
  if (!draft)
    return (
      <PageShell title={t("dashboards.draft")}>
        {editor.error ? (
          <Alert
            tone="danger"
            title={t("dashboards.failed")}
            description={formatApiError(
              editor.error,
              t("dashboards.failed"),
              (requestId) => t("common.requestReference", { requestId }),
            )}
          />
        ) : (
          t("common.loading")
        )}
      </PageShell>
    );
  return (
    <PageShell
      title={draft.name}
      description={
        <>
          <Badge>{t("dashboards.draft")}</Badge> ·{" "}
          {t(
            editor.saving
              ? "dashboards.saving"
              : editor.dirty
                ? "dashboards.unsaved"
                : "dashboards.saved",
          )}
        </>
      }
      actions={
        <div className="argus-dashboard-inline">
          <Button
            isDisabled={busy}
            onPress={() => {
              void editor
                .save()
                .then(() => navigate({ to: "/dashboards" }))
                .catch(handle);
            }}
          >
            {t("dashboards.back")}
          </Button>
          <Button
            isDisabled={busy}
            onPress={() => void editor.save().catch(handle)}
          >
            {t("dashboards.save")}
          </Button>
          <Button
            variant="primary"
            isDisabled={busy || editor.conflict}
            onPress={() => void run("preview")}
          >
            {t("dashboards.preview")}
          </Button>
        </div>
      }
    >
      {import.meta.env.VITE_API_MODE === "mock" && (
        <Alert
          tone="info"
          title={t("dashboards.sample")}
          description={t("dashboards.mock")}
        />
      )}
      {Boolean(editor.error) && (
        <Alert
          tone="danger"
          title={t("dashboards.failed")}
          description={formatApiError(
            editor.error,
            t("dashboards.failed"),
            (requestId) => t("common.requestReference", { requestId }),
          )}
        />
      )}
      {message && (
        <Alert
          tone="warning"
          title={t("dashboards.sample")}
          description={message}
        />
      )}
      <div className="argus-dashboard-toolbar">
        <div className="argus-dashboard-inline">
          <Button isDisabled={busy} onPress={() => setMetadata(true)}>
            {t("dashboards.metadata")}
          </Button>
          <Button isDisabled={busy} onPress={() => setVariables(true)}>
            {t("dashboardControls.filterManager")}
          </Button>
          <Button isDisabled={busy} onPress={() => setSettings(true)}>
            {t("dashboardControls.settings")}
          </Button>
          <Button isDisabled={busy} onPress={() => setAddingPanel(true)}>
            {t("dashboards.addPanel")}
          </Button>
          <Button
            isDisabled={busy || editor.conflict}
            onPress={() => void run("sample")}
          >
            {t("dashboards.sampleRun")}
          </Button>
        </div>
        <Button
          variant="ghost"
          isDisabled={busy}
          onPress={() => setDiscard(true)}
        >
          {t("dashboards.discard")}
        </Button>
      </div>
      <p className="argus-dashboard-muted">{t("dashboards.layoutHint")}</p>
      {sampleStatus &&
        sampleVersion === draft.draft_version &&
        !editor.dirty && (
          <p role="status">
            {t("dashboards.sample")}:{" "}
            {t(`dashboards.statuses.${sampleStatus}`, {
              defaultValue: sampleStatus,
            })}{" "}
            · {t("dashboards.sampleHint")}
          </p>
        )}
      <DashboardGrid
        items={draft.spec.panels}
        rowHeight={draft.spec.layout.row_height}
        editable={!busy && !editor.conflict}
        onChange={(panels) =>
          editor.update({ ...draft, spec: { ...draft.spec, panels } })
        }
      >
        {(p) => (
          <>
            <PanelTile
              panel={p}
              execution={
                !editor.dirty && sampleVersion === draft.draft_version
                  ? sample?.panels.find((entry) => entry.id === p.id)
                  : undefined
              }
              editable={!busy}
              onEdit={() =>
                void navigate({
                  to: "/dashboard-drafts/$draftId/panels/$panelId",
                  params: { draftId, panelId: p.id },
                })
              }
              onRemove={() => setRemove(p.id)}
            />
            <div className="argus-dashboard-panel-footer">
              <Button
                size="sm"
                variant="ghost"
                isDisabled={busy}
                onPress={() => void run("generate", p.id)}
              >
                {t("dashboards.generate")}
              </Button>
              <span>
                {t("dashboards.details")}: {p.drilldowns.length}
              </span>
            </div>
          </>
        )}
      </DashboardGrid>
      {!draft.spec.panels.length && (
        <div className="argus-dashboard-empty">
          <h2>{t("dashboards.noPanels")}</h2>
          <p>{t("dashboards.noPanelsHint")}</p>
          <Button variant="primary" onPress={() => setAddingPanel(true)}>
            {t("dashboards.addPanel")}
          </Button>
        </div>
      )}
      {addingPanel && (
        <PanelPresetPicker
          spec={draft.spec}
          onClose={() => setAddingPanel(false)}
          onChoose={(value) => {
            setAddingPanel(false);
            editor.update({
              ...draft,
              spec: { ...draft.spec, panels: [...draft.spec.panels, value] },
            });
            void editor
              .save()
              .then(() =>
                navigate({
                  to: "/dashboard-drafts/$draftId/panels/$panelId",
                  params: { draftId, panelId: value.id },
                }),
              )
              .catch(handle);
          }}
        />
      )}
      {variables && (
        <VariablesEditor
          spec={draft.spec}
          onClose={() => setVariables(false)}
          onSave={(spec) => {
            editor.update({ ...draft, spec });
            setVariables(false);
          }}
        />
      )}
      {metadata && (
        <MetadataEditor
          draft={draft}
          folders={folders.data ?? []}
          onClose={() => setMetadata(false)}
          onSave={(value) => {
            editor.update(value);
            setMetadata(false);
          }}
        />
      )}
      {settings && (
        <DashboardSettingsEditor
          spec={draft.spec}
          onClose={() => setSettings(false)}
          onSave={(spec) => {
            editor.update({ ...draft, spec });
            setSettings(false);
          }}
        />
      )}
      <ConfirmDialog
        open={Boolean(remove)}
        onOpenChange={(open) => !open && setRemove(undefined)}
        title={t("dashboards.discardPanel")}
        danger
        onConfirm={() => {
          editor.update({
            ...draft,
            spec: {
              ...draft.spec,
              panels: draft.spec.panels.filter((p) => p.id !== remove),
            },
          });
          setRemove(undefined);
        }}
      />
      <ConfirmDialog
        open={discard}
        onOpenChange={setDiscard}
        title={t("dashboards.discard")}
        description={t("dashboards.discardHint")}
        danger
        onConfirm={() => {
          void editor
            .save()
            .then((d) => api.dashboards.discardDraft(draftId, d.draft_version))
            .then(() => navigate({ to: "/dashboards" }))
            .catch(handle);
        }}
      />
      <Dialog
        open={editor.conflict}
        onOpenChange={(open) => !open && editor.setConflict(false)}
        title={t("dashboards.conflict")}
        description={t("dashboards.conflictHint")}
        size="lg"
        footer={
          <>
            <Button
              onPress={() =>
                void api.dashboards
                  .draft(draftId)
                  .then(editor.install)
                  .catch(handle)
              }
            >
              {t("dashboards.useServer")}
            </Button>
            <Button
              variant="primary"
              isDisabled={busy || !conflictDraft}
              onPress={() => void resolve()}
            >
              {t("dashboards.acknowledge")}
            </Button>
          </>
        }
      >
        <div className="argus-dashboard-diff">
          <div>
            <h3>{t("dashboards.currentPublished")}</h3>
            <pre>
              {JSON.stringify(
                published
                  ? {
                      name: published.name,
                      description: published.description,
                      spec: published.spec,
                    }
                  : conflictDraft && draftInput(conflictDraft),
                null,
                2,
              )}
            </pre>
            {published && conflictDraft && (
              <details>
                <summary>{t("dashboards.draft")}</summary>
                <pre>{JSON.stringify(draftInput(conflictDraft), null, 2)}</pre>
              </details>
            )}
          </div>
          <div>
            <h3>{t("dashboards.myChanges")}</h3>
            <pre>{JSON.stringify(draftInput(draft), null, 2)}</pre>
          </div>
        </div>
      </Dialog>
      {action && (
        <DashboardActionDialog
          action={action}
          onClose={() => setAction(null)}
          onError={(e) => {
            handle(e);
            return true;
          }}
          onDone={() => {
            void api.dashboards
              .draft(draftId)
              .then((d) =>
                d.dashboard_id
                  ? navigate({
                      to: "/dashboards/$dashboardId",
                      params: { dashboardId: d.dashboard_id },
                    })
                  : navigate({ to: "/dashboards" }),
              )
              .catch(handle);
          }}
        />
      )}
    </PageShell>
  );
}
function MetadataEditor({
  draft,
  folders,
  onSave,
  onClose,
}: {
  draft: import("@argus/api-client").DashboardDraft;
  folders: import("@argus/api-client").DashboardFolder[];
  onSave: (draft: import("@argus/api-client").DashboardDraft) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [value, setValue] = useState(draft);
  const form = useDefinitionForm(
    value,
    (d) =>
      Boolean(d.name.trim()) &&
      d.name.length <= 180 &&
      d.description.length <= 2048,
    t("dashboards.editor.invalidForm"),
  );
  return (
    <FormDrawer
      open
      title={t("dashboards.metadata")}
      onOpenChange={(open) => !open && onClose()}
      submitLabel={t("dashboards.done")}
      onSubmit={form.handleSubmit(onSave)}
    >
      <div className="argus-dashboard-form-stack">
        <Field
          label={t("dashboards.name")}
          requirement="required"
          error={form.error}
        >
          <Input
            value={value.name}
            maxLength={120}
            onChange={(e) => setValue({ ...value, name: e.target.value })}
          />
        </Field>
        <Field label={t("dashboards.descriptionField")} requirement="optional">
          <Textarea
            value={value.description}
            onChange={(e) =>
              setValue({ ...value, description: e.target.value })
            }
          />
        </Field>
        <Field label={t("dashboards.folder")} requirement="optional">
          <Select
            value={value.folder_id ?? ""}
            onValueChange={(id) =>
              setValue({ ...value, folder_id: id || undefined })
            }
            options={[
              { value: "", label: t("dashboards.ungrouped") },
              ...folders
                .filter((f) => f.status === "active")
                .map((f) => ({ value: f.id, label: f.name })),
            ]}
          />
        </Field>
      </div>
    </FormDrawer>
  );
}
