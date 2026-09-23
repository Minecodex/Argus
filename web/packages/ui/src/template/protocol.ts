export const TEMPLATE_PROTOCOL = "argus-template/v1";
export const TEMPLATE_MAX_BYTES = 2 * 1024 * 1024;
export type TemplateContext = {
  locale: "zh-CN" | "en-US";
  color_scheme: "light" | "dark";
  tokens: Record<string, string>;
};
export type TemplateResource = { type: string; id: string };
export type TemplatePresentation = {
  template_source: string;
  template_hash: string;
  detail_data: Record<string, unknown>;
  resource_refs?: TemplateResource[];
  result_refs?: string[];
};
export type TemplateMessage = {
  version: typeof TEMPLATE_PROTOCOL;
  nonce: string;
  sequence: number;
  request_id: string;
  type: string;
  payload: Record<string, unknown>;
};
const messageTypes = new Set([
  "hello",
  "ready",
  "context",
  "resize",
  "open_resource",
  "open_result",
  "error",
  "destroy",
]);
export function messageBytes(value: unknown): number {
  try {
    return new TextEncoder().encode(JSON.stringify(value)).length;
  } catch {
    return Infinity;
  }
}
export function isTemplateMessage(value: unknown): value is TemplateMessage {
  if (
    !value ||
    typeof value !== "object" ||
    Array.isArray(value) ||
    messageBytes(value) > TEMPLATE_MAX_BYTES
  )
    return false;
  const item = value as Record<string, unknown>;
  return (
    item.version === TEMPLATE_PROTOCOL &&
    typeof item.nonce === "string" &&
    /^[A-Za-z0-9_-]{32,128}$/.test(item.nonce) &&
    Number.isSafeInteger(item.sequence) &&
    Number(item.sequence) > 0 &&
    typeof item.request_id === "string" &&
    item.request_id.length >= 16 &&
    typeof item.type === "string" &&
    messageTypes.has(item.type) &&
    !!item.payload &&
    typeof item.payload === "object" &&
    !Array.isArray(item.payload) &&
    validPayload(String(item.type), item.payload as Record<string, unknown>)
  );
}
export async function templateHash(source: string): Promise<string> {
  const hash = await crypto.subtle.digest(
    "SHA-256",
    new TextEncoder().encode(source),
  );
  return Array.from(new Uint8Array(hash), (value) =>
    value.toString(16).padStart(2, "0"),
  ).join("");
}

function validPayload(type: string, payload: Record<string, unknown>): boolean {
  const keys = Object.keys(payload);
  if (type === "ready" || type === "destroy") return keys.length === 0;
  if (type === "resize")
    return (
      keys.length === 1 &&
      typeof payload.height === "number" &&
      Number.isFinite(payload.height) &&
      payload.height >= 1 &&
      payload.height <= 2147483647
    );
  if (type === "open_resource")
    return (
      keys.length === 2 &&
      typeof payload.type === "string" &&
      payload.type.length <= 64 &&
      typeof payload.id === "string" &&
      payload.id.length <= 128
    );
  if (type === "open_result")
    return (
      keys.length === 1 &&
      typeof payload.ref === "string" &&
      payload.ref.length <= 128
    );
  if (type === "error")
    return keys.length === 1 && typeof payload.code === "string";
  return true;
}
