import { useEffect, useMemo, useState } from "react";
import { Button } from "./button";
import { CodeBlock } from "./code";
import { useUiText } from "./locale";
import { Markdown } from "./markdown";
import { DataTable } from "./table";
import { filePreviewKind } from "./file-preview-format";
import {
  FILE_PREVIEW_COLUMNS,
  FILE_PREVIEW_ROWS,
  formatJsonPreview,
  parseDelimited,
} from "./file-preview-data";

export function FileTextPreview({
  name,
  content,
  mediaType,
  path,
}: {
  name: string;
  content: string;
  mediaType?: string;
  path?: string;
}) {
  const text = useUiText();
  const [raw, setRaw] = useState(false);
  const [header, setHeader] = useState(true);
  const kind = filePreviewKind(name, mediaType, path);
  const markdownLimited = kind === "markdown" && content.length > 200000;
  const parsed = useMemo(() => {
    try {
      if (kind === "csv" || kind === "tsv")
        return { table: parseDelimited(content, kind === "csv" ? "," : "\t") };
      if (kind === "json") return { formatted: formatJsonPreview(content) };
      if (kind === "jsonl") {
        const lines = content.split(/\r?\n/).filter((line) => line.trim());
        return {
          formatted: lines
            .slice(0, FILE_PREVIEW_ROWS)
            .map(formatJsonPreview)
            .join("\n\n"),
          limited: lines.length > FILE_PREVIEW_ROWS,
        };
      }
      return {};
    } catch {
      return { malformed: true };
    }
  }, [kind, content]);
  if (!content.trim())
    return <p role="status">{text("文件为空。", "The file is empty.")}</p>;
  const table = parsed.table;
  const body =
    table?.rows.slice(
      header ? 1 : 0,
      header ? FILE_PREVIEW_ROWS + 1 : FILE_PREVIEW_ROWS,
    ) ?? [];
  const width = table ? Math.max(0, ...table.rows.map((row) => row.length)) : 0;
  return (
    <section
      className="argus-file-preview"
      aria-label={text("文件内容", "File content")}
    >
      <div className="argus-file-preview__toolbar">
        <div className="argus-file-preview__modes">
          <Button
            size="sm"
            variant="ghost"
            aria-pressed={!raw}
            onPress={() => setRaw(false)}
          >
            {text("预览", "Preview")}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            aria-pressed={raw}
            onPress={() => setRaw(true)}
          >
            {text("原文", "Raw")}
          </Button>
        </div>
        {table && !raw && (
          <Button
            size="sm"
            variant="ghost"
            aria-pressed={header}
            onPress={() => setHeader(!header)}
          >
            {text("首行作为表头", "First row as headers")}
          </Button>
        )}
      </div>
      {parsed.malformed && !raw && (
        <p role="status" className="argus-file-preview__notice">
          {text(
            "内容无法按声明格式解析，以下显示原文。",
            "The declared format could not be parsed. Original text is shown below.",
          )}
        </p>
      )}
      {markdownLimited && !raw && (
        <p role="status" className="argus-file-preview__notice">
          {text(
            "Markdown 超过渲染预算，以下显示原文；可下载完整文件。",
            "Markdown exceeds the rendering budget. Original text is shown; the complete file can be downloaded.",
          )}
        </p>
      )}
      {!raw &&
        (table?.rowsLimited ||
          (table && table.rows.length - (header ? 1 : 0) > FILE_PREVIEW_ROWS) ||
          table?.columnsLimited ||
          table?.cellsLimited ||
          parsed.limited) && (
          <p role="status" className="argus-file-preview__notice">
            {text(
              `预览最多 ${FILE_PREVIEW_ROWS} 行、${FILE_PREVIEW_COLUMNS} 列，超长单元格已截断；原文或下载可查看完整内容。`,
              `Preview is limited to ${FILE_PREVIEW_ROWS} rows and ${FILE_PREVIEW_COLUMNS} columns; long cells are truncated. Use Raw or download for the complete content.`,
            )}
          </p>
        )}
      {raw ? (
        <pre className="argus-file-preview__text">{content}</pre>
      ) : table ? (
        <>
          <DataTable
            className="argus-file-preview__table"
            density="compact"
            columns={Array.from({ length: width }, (_, index) => ({
              key: String(index),
              header: header
                ? table.rows[0]?.[index] ||
                  text(`列 ${index + 1}`, `Column ${index + 1}`)
                : text(`列 ${index + 1}`, `Column ${index + 1}`),
              render: (row: { cells: string[] }) => (
                <span className="argus-file-preview__cell">
                  {row.cells[index] ?? ""}
                </span>
              ),
            }))}
            data={body.map((cells, index) => ({ id: String(index), cells }))}
            getRowKey={(row) => row.id}
          />
          {!body.length && (
            <p role="status">
              {text("没有数据行。", "There are no data rows.")}
            </p>
          )}
        </>
      ) : kind === "markdown" && !markdownLimited ? (
        <Markdown>{content}</Markdown>
      ) : kind === "json" || kind === "jsonl" ? (
        <pre className="argus-file-preview__text">
          {parsed.formatted ?? content}
        </pre>
      ) : kind === "code" ? (
        <CodeBlock code={content} label={name} />
      ) : (
        <pre className="argus-file-preview__text">{content}</pre>
      )}
    </section>
  );
}

export function FileImagePreview({ blob, name }: { blob: Blob; name: string }) {
  const [url, setUrl] = useState<string>();
  const [failed, setFailed] = useState(false);
  const text = useUiText();
  useEffect(() => {
    setFailed(false);
    const value = URL.createObjectURL(blob);
    setUrl(value);
    return () => URL.revokeObjectURL(value);
  }, [blob]);
  return (
    <div className="argus-file-preview__image">
      {failed ? (
        <p role="alert">
          {text(
            "图片无法解码，请下载查看。",
            "The image could not be decoded. Please download it to inspect.",
          )}
        </p>
      ) : (
        url && <img src={url} alt={name} onError={() => setFailed(true)} />
      )}
    </div>
  );
}
