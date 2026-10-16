export const TEMPLATE_PROTOCOL = "argus-template/v1";
export const TEMPLATE_PROTOCOL_V2 = "argus-template/v2";
export type TemplateVersion =
  typeof TEMPLATE_PROTOCOL | typeof TEMPLATE_PROTOCOL_V2;
export type TemplateReadOperation =
  | "resources.context"
  | "metric.context"
  | "alert.details"
  | "dashboard.result"
  | "result.page"
  | "runtime.status"
  | "result.detail";
export type TemplateReadInput = {
  operation: TemplateReadOperation;
  parameters: Record<string, unknown>;
};
export type TemplateReadResult = {
  status: "ready" | "pending";
  data: Record<string, unknown>;
  next_cursor?: string;
};
export const TEMPLATE_MAX_BYTES = 2 * 1024 * 1024;
export type TemplateContext = {
  locale: "zh-CN" | "en-US";
  color_scheme: "light" | "dark";
  layout?: "preview" | "document";
  tokens: Record<string, string>;
};
export type TemplateResource = { type: string; id: string };
export type TemplatePresentation = {
  runtime?: TemplateVersion;
  read_operations?: string[];
  template_source: string;
  template_hash: string;
  detail_data: Record<string, unknown>;
  resource_refs?: TemplateResource[];
  result_refs?: string[];
};
export type TemplateMessage = {
  version: TemplateVersion;
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
  "data_request",
  "data_response",
  "data_available",
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
    (item.version === TEMPLATE_PROTOCOL ||
      item.version === TEMPLATE_PROTOCOL_V2) &&
    typeof item.nonce === "string" &&
    /^[A-Za-z0-9_-]{32,128}$/.test(item.nonce) &&
    Number.isSafeInteger(item.sequence) &&
    Number(item.sequence) > 0 &&
    typeof item.request_id === "string" &&
    item.request_id.length >= 16 &&
    item.request_id.length <= 128 &&
    typeof item.type === "string" &&
    messageTypes.has(item.type) &&
    (item.version === TEMPLATE_PROTOCOL_V2 ||
      !["data_request", "data_response", "data_available"].includes(
        item.type,
      )) &&
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
  if (type === "data_request")
    return (
      keys.length === 2 &&
      typeof payload.operation === "string" &&
      payload.operation.length <= 64 &&
      !!payload.parameters &&
      typeof payload.parameters === "object" &&
      !Array.isArray(payload.parameters)
    );
  if (type === "data_response")
    return (
      (typeof payload.error === "string" && keys.length === 1) ||
      (keys.length === 1 &&
        !!payload.result &&
        typeof payload.result === "object" &&
        !Array.isArray(payload.result))
    );
  if (type === "data_available") return keys.length === 0;
  return true;
}
