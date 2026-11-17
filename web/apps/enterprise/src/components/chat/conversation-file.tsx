import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { ApiError, formatApiError, useApi } from "@argus/api-client";
import {
  Dialog,
  FileCard,
  FileRow,
  FileTextPreview,
  FileImagePreview,
  filePreviewKind,
  readFilePreview,
  FilePreviewContentError,
} from "@argus/ui";
import type { ChatFile } from "./chat-view-model";

export function ConversationFile({
  file,
  conversationId,
  kind = "file",
  layout = "card",
  name,
}: {
  file: ChatFile;
  conversationId: string;
  kind?: "file" | "delivery";
  layout?: "card" | "row";
  name?: string;
}) {
  const api = useApi();
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  let href: string | undefined;
  try {
    href = api.workspace.downloadUrl(conversationId, file.id, kind);
  } catch {
    href = undefined;
  }
  const queryJobId = file.path?.match(/^\/workspace\/queries\/([^/]+)\//)?.[1];
  const queryJob = useQuery({
    queryKey: ["chat-query-file-label", conversationId, queryJobId],
    queryFn: () => api.dashboardQueries.get(conversationId, queryJobId!),
    enabled: !!queryJobId,
    retry: false,
  });
  const storedFile = queryJob.data?.files?.find(
    (item) => item.workspace_file_id === file.id,
  );
  const definition = queryJob.data?.manifest?.definition as
    Record<string, unknown> | undefined;
  const spec = definition?.spec as Record<string, unknown> | undefined;
  const panels = (definition?.panels ?? spec?.panels) as
    Array<{ id: string; title?: string }> | undefined;
  const panelTitle = panels?.find(
    (item) => item.id === storedFile?.panel_id,
  )?.title;
  const displayName = storedFile
    ? storedFile.kind === "manifest"
      ? `${t("chat.files.queryManifest")}.json`
      : `${panelTitle ?? storedFile.panel_id ?? t("chat.files.queryData")} · ${t("chat.files.queryData")}.json`
    : file.name;
  const format = filePreviewKind(file.name, file.media_type, file.path);
  const previewable = format !== "unsupported";
  const content = useQuery({
    queryKey: [
      "workspace-preview",
      conversationId,
      kind,
      file.id,
      file.content_hash,
    ],
    queryFn: async ({ signal }) =>
      readFilePreview(
        await api.workspace.readContent(conversationId, file.id, kind, signal),
        format,
        file.media_type,
      ),
    enabled: open && previewable,
    retry: false,
    staleTime: 0,
    gcTime: 0,
  });
  const isQuery = file.path?.startsWith("/workspace/queries/");
  return (
    <>
      {layout === "row" ? (
        <FileRow
          name={name ?? file.name}
          path={file.path}
          size={file.byte_size}
          description={isQuery ? displayName : file.media_type}
          downloadHref={href}
          downloadName={file.name}
          onPreview={() => setOpen(true)}
        />
      ) : (
        <FileCard
          name={displayName}
          size={file.byte_size}
          description={isQuery ? t("chat.files.queryData") : file.media_type}
          downloadHref={href}
          onPreview={() => setOpen(true)}
        />
      )}
      <Dialog
        open={open}
        onOpenChange={setOpen}
        size="cover"
        title={displayName}
        description={t("chat.files.previewDescription")}
        footer={
          href && (
            <a
              className="argus-file-card__download"
              href={href}
              download={file.name}
            >
              {t("chat.files.download")}
            </a>
          )
        }
      >
        {!previewable ? (
          <p>{t("chat.files.previewUnsupported")}</p>
        ) : content.isError ? (
          <p role="alert">
            {content.error instanceof FilePreviewContentError
              ? t("chat.files.previewInvalidContent")
              : content.error instanceof ApiError &&
                  content.error.code === "WORKSPACE_PREVIEW_TOO_LARGE"
                ? t("chat.files.previewTooLarge")
                : formatApiError(
                    content.error,
                    t("chat.files.previewFailed"),
                    (id) => t("common.requestReference", { requestId: id }),
                  )}
          </p>
        ) : content.isPending || content.isFetching ? (
          <p role="status">{t("chat.files.loading")}</p>
        ) : content.data?.kind === "image" ? (
          <FileImagePreview blob={content.data.blob} name={file.name} />
        ) : (
          <FileTextPreview
            key={file.id}
            name={file.name}
            path={file.path}
            mediaType={file.media_type}
            content={content.data?.text ?? ""}
          />
        )}
      </Dialog>
    </>
  );
}

export function MessageFiles({
  items,
  conversationId,
  kind = "file",
  group,
  compact = false,
}: {
  items: ChatFile[];
  conversationId: string;
  kind?: "file" | "delivery";
  group?: string;
  compact?: boolean;
}) {
  const { t } = useTranslation();
  const cards = (
    <div className="argus-chat-files">
      {items.map((file) => (
        <ConversationFile
          key={file.id}
          file={file}
          conversationId={conversationId}
          kind={kind}
          layout={compact ? "row" : "card"}
        />
      ))}
    </div>
  );
  return group && items.length > 1 ? (
    <details className="argus-chat-files-group">
      <summary>{t("chat.files.queryGroup", { count: items.length })}</summary>
      {cards}
    </details>
  ) : (
    cards
  );
}
