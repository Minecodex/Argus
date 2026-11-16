import type { FilePreviewKind } from "./file-preview-format";

export class FilePreviewContentError extends Error {}

async function bytes(blob: Blob): Promise<ArrayBuffer> {
  if (typeof blob.arrayBuffer === "function") return blob.arrayBuffer();
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as ArrayBuffer);
    reader.onerror = () => reject(reader.error);
    reader.readAsArrayBuffer(blob);
  });
}

function rasterMime(data: Uint8Array): string | undefined {
  const ascii = (start: number, end: number) =>
    String.fromCharCode(...data.slice(start, end));
  if (
    data[0] === 137 &&
    ascii(1, 4) === "PNG" &&
    data[4] === 13 &&
    data[5] === 10 &&
    data[6] === 26 &&
    data[7] === 10
  )
    return "image/png";
  if (data[0] === 255 && data[1] === 216 && data[2] === 255)
    return "image/jpeg";
  if (/^GIF8[79]a$/.test(ascii(0, 6))) return "image/gif";
  if (ascii(0, 4) === "RIFF" && ascii(8, 12) === "WEBP") return "image/webp";
  if (ascii(4, 8) === "ftyp" && /avi[fs]/.test(ascii(8, 32)))
    return "image/avif";
  if (ascii(0, 2) === "BM") return "image/bmp";
  if (data[0] === 0 && data[1] === 0 && data[2] === 1 && data[3] === 0)
    return "image/x-icon";
}

/** Decode only after the authorized bounded fetch; never execute HTML/SVG or guess a legacy encoding. */
export async function readFilePreview(
  blob: Blob,
  kind: FilePreviewKind,
  mediaType?: string,
) {
  const buffer = await bytes(blob);
  const data = new Uint8Array(buffer);
  if (kind === "image") {
    const mime = rasterMime(data);
    if (!mime) throw new FilePreviewContentError("Raster signature mismatch");
    return { kind: "image" as const, blob: new Blob([buffer], { type: mime }) };
  }
  let encoding =
    (mediaType ?? blob.type).match(/charset\s*=\s*["']?([^\s;"']+)/i)?.[1] ??
    "utf-8";
  if (data[0] === 255 && data[1] === 254) encoding = "utf-16le";
  else if (data[0] === 254 && data[1] === 255) encoding = "utf-16be";
  else if (data[0] === 239 && data[1] === 187 && data[2] === 191)
    encoding = "utf-8";
  try {
    const text = new TextDecoder(encoding, { fatal: true }).decode(data);
    if (text.includes("\0")) throw new Error("Binary content");
    return { kind: "text" as const, text };
  } catch {
    throw new FilePreviewContentError(
      "Unsupported encoding or non-text content",
    );
  }
}
