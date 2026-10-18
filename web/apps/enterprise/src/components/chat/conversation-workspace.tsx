import { Checkbox, FileTree } from "@argus/ui";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useApi, formatApiError, ApiError } from "@argus/api-client";
import { useEnterpriseAuthStore } from "@argus/auth";
import "../../styles/planv5.css";
import { ConversationFile } from "./conversation-file";

export function ConversationWorkspace({
  conversationId,
  selected,
  disabled,
  expanded = false,
}: {
  conversationId: string;
  selected: string[];
  disabled: boolean;
  expanded?: boolean;
}) {
  const api = useApi();
  const { t, i18n } = useTranslation();
  const queries = useQueryClient();
  const userId = useEnterpriseAuthStore((state) => state.session?.user.id);
  const connections = useQuery({
    queryKey: ["mcp-connections"],
    queryFn: () => api.mcp.list(),
  });
  const files = useQuery({
    queryKey: ["workspace-files", conversationId],
    queryFn: async () => {
      try {
        return await api.workspace.listFiles(conversationId);
      } catch (error) {
        if (
          error instanceof ApiError &&
          error.status === 404 &&
          error.code === "WORKSPACE_NOT_FOUND"
        )
          return [];
        throw error;
      }
    },
    retry: false,
  });
  const [busy, setBusy] = useState(false);
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
  const granted = (connections.data ?? []).filter((item) =>
    item.member_ids.includes(userId ?? ""),
  );
  return (
    <div className="argus-conversation-workspace">
      <details className="argus-planv5-selector" open={expanded}>
        <summary>
          {t("planv5.mcp.select")} · {selected.length}
        </summary>
        <p>{t("planv5.mcp.selectionHint")}</p>
        {granted.length === 0 && <p>{t("planv5.mcp.empty")}</p>}
        {granted.map((item) => (
          <Checkbox
            className="argus-planv5-connection"
            key={item.id}
            isSelected={selected.includes(item.id)}
            isDisabled={
              disabled ||
              busy ||
              (item.status === "disabled" && !selected.includes(item.id))
            }
            onChange={(selected) => void setSelection(item.id, selected)}
          >
            {item.name}· {item.tool_count}
            {t("planv5.mcp.tools")}
            {item.status === "disabled" && ` · ${t("planv5.mcp.disabled")}`}
          </Checkbox>
        ))}
        {selected
          .filter((id) => !granted.some((item) => item.id === id))
          .map((id) => (
            <Checkbox
              className="argus-planv5-connection"
              key={id}
              isSelected
              isDisabled={disabled || busy}
              onChange={() => void setSelection(id, false)}
            >
              {t("planv5.mcp.disabled")}
            </Checkbox>
          ))}
      </details>
      <details open={expanded}>
        <summary>
          {t("planv5.files.workspace")} · {files.data?.length ?? 0}
        </summary>
        <p>{t("planv5.files.retention")}</p>
        {!!files.data?.length && (
          <FileTree
            key={conversationId}
            files={files.data}
            label={t("planv5.files.workspace")}
            locale={i18n.language}
            renderFile={(file, name) => (
              <ConversationFile
                file={file}
                conversationId={conversationId}
                layout="row"
                name={name}
              />
            )}
          />
        )}
        {files.isPending && <p role="status">{t("chat.files.loading")}</p>}
        {files.isSuccess && !files.data.length && (
          <p>{t("planv5.files.empty")}</p>
        )}
        {files.isError && <p role="alert">{t("planv5.files.unavailable")}</p>}
      </details>
      {error && <p role="alert">{error}</p>}
    </div>
  );
}
