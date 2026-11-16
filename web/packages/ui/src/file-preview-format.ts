export type FilePreviewKind =
  | "markdown"
  | "csv"
  | "tsv"
  | "json"
  | "jsonl"
  | "code"
  | "text"
  | "image"
  | "unsupported";

const raster =
  /^(image\/(png|jpeg|gif|webp|avif|bmp|x-icon|vnd\.microsoft\.icon))$/;
const code =
  /\.(ya?ml|toml|xml|html?|svg|sql|sh|bash|zsh|ps1|py|js|jsx|mjs|cjs|ts|tsx|css|go|rs|java|c|cpp|h|rb|php|graphql|gql|ini|conf|config)$/i;
const codeMime =
  /^application\/(xml|javascript|ecmascript|typescript|yaml|x-yaml|vnd\.yaml|toml|sql|x-sql|graphql|x-sh|x-shellscript)$/;

/** Metadata is authoritative: a declared binary cannot become text by renaming it. */
export function filePreviewKind(
  name: string,
  mediaType?: string,
  path?: string,
): FilePreviewKind {
  const mime = mediaType?.split(";")[0]?.trim().toLowerCase();
  if (mime && raster.test(mime)) return "image";
  if (mime === "image/svg+xml") return "code";
  if (mime === "text/markdown" || mime === "text/x-markdown") return "markdown";
  if (mime === "text/csv" || mime === "application/csv") return "csv";
  if (mime === "text/tab-separated-values") return "tsv";
  if (mime && /^(application|text)\/(x-ndjson|ndjson|jsonl)$/.test(mime))
    return "jsonl";
  if (
    mime &&
    (/^(application|text)\/json$/.test(mime) ||
      /^application\/[\w.-]+\+json$/.test(mime))
  )
    return "json";
  if (
    mime &&
    !mime.startsWith("text/") &&
    !codeMime.test(mime) &&
    !/^application\/[\w.-]+\+xml$/.test(mime)
  )
    return "unsupported";
  const filename = /\.[a-z\d]+$/i.test(name) ? name : (path ?? name);
  if (/\.(md|markdown|mdown)$/i.test(filename)) return "markdown";
  if (/\.csv$/i.test(filename)) return "csv";
  if (/\.tsv$/i.test(filename)) return "tsv";
  if (/\.(jsonl|ndjson)$/i.test(filename)) return "jsonl";
  if (/\.json$/i.test(filename)) return "json";
  if (
    code.test(filename) ||
    (mime && (codeMime.test(mime) || mime.endsWith("+xml"))) ||
    mime === "text/html"
  )
    return "code";
  if (mime?.startsWith("text/") || /\.(txt|log|out|err)$/i.test(filename))
    return "text";
  if (!mime && /\.(png|jpe?g|gif|webp|avif|bmp|ico)$/i.test(filename))
    return "image";
  return "unsupported";
}
