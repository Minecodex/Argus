import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useApi, formatApiError, ApiError } from "@argus/api-client";
import { useEnterpriseAuthStore } from "@argus/auth";
import { Button, ConfirmDialog } from "@argus/ui";
import "../../styles/planv5.css";

export function ConversationWorkspace({
  conversationId,
  selected,
  disabled,
}: {
  conversationId: string;
  selected: string[];
  disabled: boolean;
}) {
  const api = useApi();
  const { t } = useTranslation();
  const queries = useQueryClient();
  const userId = useEnterpriseAuthStore((state) => state.session?.user.id);
  const connections = useQuery({
    queryKey: ["mcp-connections"],
    queryFn: () => api.mcp.list(),
  });
  const files = useQuery({
    queryKey: ["workspace-files", conversationId],
    queryFn: () => api.workspace.listFiles(conversationId),
    retry: false,
  });
  const workspace = useQuery({
    queryKey: ["workspace", conversationId],
    queryFn: async () => {
      try {
        return await api.workspace.get(conversationId);
      } catch (error) {
        if (error instanceof ApiError && error.status === 404) return null;
        throw error;
      }
    },
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.status === "deleting" ||
      query.state.data?.status === "provisioning"
        ? 1_000
        : false,
  });
  const [busy, setBusy] = useState(false);
  const [remove, setRemove] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const setSelection = async (id: string, checked: boolean) => {
    setBusy(true);
    setError(null);
    try {
      await api.conversations.updateConnections(
        conversationId,
        checked ? [...selected, id] : selected.filter((value) => value !== id),
      );
      await queries.invalidateQueries({
        queryKey: ["conversations", "detail", conversationId],
      });
    } catch (error) {
      setError(
        formatApiError(error, t("planv5.mcp.failed"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      setBusy(false);
    }
  };
  const deleteWorkspace = async () => {
    setBusy(true);
    setError(null);
    try {
      const deleting = await api.workspace.remove(conversationId);
      queries.setQueryData(["workspace", conversationId], deleting);
      setRemove(false);
      await queries.invalidateQueries({
        queryKey: ["workspace-files", conversationId],
      });
      await queries.invalidateQueries({
        queryKey: ["workspace", conversationId],
      });
      await queries.invalidateQueries({
        queryKey: ["conversations", conversationId],
      });
    } catch (error) {
      setError(
        formatApiError(error, t("planv5.files.failed"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      setBusy(false);
    }
  };
  const granted = (connections.data ?? []).filter((item) =>
    item.member_ids.includes(userId ?? ""),
  );
  return (
    <div className="argus-conversation-workspace">
      <details className="argus-planv5-selector">
        <summary>
          {t("planv5.mcp.select")} · {selected.length}
        </summary>
        <p>{t("planv5.mcp.selectionHint")}</p>
        {granted.length === 0 && <p>{t("planv5.mcp.empty")}</p>}
        {granted.map((item) => (
          <label className="argus-planv5-connection" key={item.id}>
            <input
              type="checkbox"
              checked={selected.includes(item.id)}
              disabled={
                disabled ||
                busy ||
                (item.status === "disabled" && !selected.includes(item.id))
              }
              onChange={(event) =>
                void setSelection(item.id, event.target.checked)
              }
            />
            {item.name} · {item.tool_count} {t("planv5.mcp.tools")}
            {item.status === "disabled" && ` · ${t("planv5.mcp.disabled")}`}
          </label>
        ))}
        {selected
          .filter((id) => !granted.some((item) => item.id === id))
          .map((id) => (
            <label className="argus-planv5-connection" key={id}>
              <input
                type="checkbox"
                checked
                disabled={disabled || busy}
                onChange={() => void setSelection(id, false)}
              />
              {t("planv5.mcp.disabled")}
            </label>
          ))}
      </details>
      <details>
        <summary>
          {t("planv5.files.workspace")} · {files.data?.length ?? 0}
        </summary>
        <p>{t("planv5.files.retention")}</p>
        {(files.data ?? []).map((file) => (
          <a
            className="argus-chat-file"
            href={api.workspace.downloadUrl(conversationId, file.id, "file")}
            download={file.name}
            key={file.id}
          >
            {file.name}
          </a>
        ))}
        {!files.data?.length && <p>{t("planv5.files.empty")}</p>}
        {workspace.isError && (
          <p role="alert">{t("planv5.files.unavailable")}</p>
        )}
        {workspace.data?.status === "deleting" && (
          <p role="status">{t("planv5.files.deleting")}</p>
        )}
        <Button
          variant="ghost"
          size="sm"
          disabled={
            busy ||
            disabled ||
            workspace.isError ||
            !workspace.data ||
            workspace.data.status === "deleted" ||
            workspace.data.status === "deleting"
          }
          onClick={() => setRemove(true)}
        >
          {t("planv5.files.delete")}
        </Button>
      </details>
      {error && <p role="alert">{error}</p>}
      <ConfirmDialog
        open={remove}
        onOpenChange={setRemove}
        title={t("planv5.files.delete")}
        description={t("planv5.files.deleteConfirm")}
        confirmLabel={t("planv5.files.delete")}
        danger
        loading={busy}
        onConfirm={() => void deleteWorkspace()}
      />
    </div>
  );
}
