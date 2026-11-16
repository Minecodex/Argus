import { Download, Eye, FileText } from "lucide-react";
import { Button } from "./button";
import { useUiText } from "./locale";
export { FileTextPreview, FileImagePreview } from "./file-preview";
export { filePreviewKind } from "./file-preview-format";
export {
  readFilePreview,
  FilePreviewContentError,
} from "./file-preview-content";

export function FileCard({
  name,
  size,
  description,
  downloadHref,
  onPreview,
  disabled = false,
}: {
  name: string;
  size?: number;
  description?: string;
  downloadHref?: string;
  onPreview: () => void;
  disabled?: boolean;
}) {
  const text = useUiText();
  return (
    <article className="argus-file-card">
      <FileText aria-hidden className="argus-file-card__icon" />
      <div className="argus-file-card__content">
        <button
          className="argus-file-card__name"
          type="button"
          disabled={disabled}
          onClick={onPreview}
        >
          {name}
        </button>
        <small>
          {description}
          {description && size !== undefined ? " · " : ""}
          {size !== undefined &&
            (size < 1024 ? `${size} B` : `${(size / 1024).toFixed(1)} KB`)}
        </small>
        <div className="argus-file-card__actions">
          <Button
            variant="ghost"
            size="sm"
            onPress={onPreview}
            isDisabled={disabled}
            aria-label={`${text("预览", "Preview")} ${name}`}
          >
            <Eye size={14} />
            {text("预览", "Preview")}
          </Button>
          {downloadHref && (
            <a
              className="argus-file-card__download"
              href={downloadHref}
              download={name}
              aria-label={`${text("下载", "Download")} ${name}`}
            >
              <Download size={14} />
              {text("下载", "Download")}
            </a>
          )}
        </div>
      </div>
    </article>
  );
}
