import { Download, FileText } from "lucide-react";
import { Button } from "./button";
import { useUiText } from "./locale";

export function FileRow({
  name,
  path,
  size,
  description,
  downloadHref,
  downloadName = name,
  onPreview,
}: {
  name: string;
  path?: string;
  size?: number;
  description?: string;
  downloadHref?: string;
  downloadName?: string;
  onPreview(): void;
}) {
  const text = useUiText();
  const previewLabel = `${text("预览", "Preview")} ${name}`;
  const downloadLabel = `${text("下载", "Download")} ${name}`;
  return (
    <div
      className="argus-file-row"
      title={[path ?? name, description].filter(Boolean).join(" · ")}
    >
      <Button
        className="argus-file-row__preview"
        variant="ghost"
        size="sm"
        onPress={onPreview}
        aria-label={previewLabel}
      >
        <FileText aria-hidden />
        <span className="argus-file-tree__name">{name}</span>
      </Button>
      {size !== undefined && (
        <small className="argus-file-row__size">
          {size < 1024 ? `${size} B` : `${(size / 1024).toFixed(1)} KB`}
        </small>
      )}
      {downloadHref && (
        <a
          className="argus-file-row__download"
          href={downloadHref}
          download={downloadName}
          aria-label={downloadLabel}
          title={downloadLabel}
        >
          <Download aria-hidden />
        </a>
      )}
    </div>
  );
}
