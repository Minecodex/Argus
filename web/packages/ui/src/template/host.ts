import {
  TEMPLATE_PROTOCOL,
  TEMPLATE_MAX_BYTES,
  isTemplateMessage,
  messageBytes,
  type TemplateContext,
  type TemplatePresentation,
  type TemplateResource,
  type TemplateReadInput,
  type TemplateReadResult,
  type TemplateReadOperation,
} from "./protocol";
import { scheduleTemplateLoad } from "./load-queue";

export type TemplateHostOptions = {
  origin: string;
  presentation: TemplatePresentation;
  context: TemplateContext;
  onReady(): void;
  onError(reason: string): void;
  onResize(height: number): void;
  onResource?(ref: TemplateResource): void;
  onResult?(ref: string): void;
  onRead?(input: TemplateReadInput): Promise<TemplateReadResult>;
};

export function createTemplateHost(
  iframe: HTMLIFrameElement,
  options: TemplateHostOptions,
) {
  const target = new URL(options.origin);
  if (
    !["http:", "https:"].includes(target.protocol) ||
    target.origin === window.location.origin ||
    target.username ||
    target.password
  )
    throw new Error("Template runtime requires its own HTTP origin");
  const nonce = crypto.randomUUID();
  const version = options.presentation.runtime ?? TEMPLATE_PROTOCOL;
  let outgoing = 0,
    incoming = 0,
    destroyed = false;
  let port: MessagePort | undefined;
  let context = options.context;
  const requests = new Set<string>();
  let activeReads = 0;
  const envelope = (
    type: string,
    payload: Record<string, unknown>,
    requestId?: string,
  ) => ({
    version,
    nonce,
    sequence: ++outgoing,
    request_id: requestId ?? crypto.randomUUID(),
    type,
    payload,
  });
  const send = (type: string, payload: Record<string, unknown>) => {
    if (!destroyed && port) port.postMessage(envelope(type, payload));
  };
  const reject = () => {
    options.onError("TEMPLATE_PROTOCOL_REJECTED");
    destroy();
  };
  let timeout: number | undefined;
  let releaseLoad: (() => void) | undefined;
  const connect = () => {
    if (destroyed || port || !iframe.contentWindow) return;
    const channel = new MessageChannel();
    port = channel.port1;
    port.onmessage = (event: MessageEvent<unknown>) => {
      if (destroyed) return;
      if (
        !isTemplateMessage(event.data) ||
        event.data.nonce !== nonce ||
        event.data.version !== version ||
        event.data.sequence <= incoming
      ) {
        reject();
        return;
      }
      const { type, payload, sequence } = event.data;
      incoming = sequence;
      if (type === "ready") {
        window.clearTimeout(timeout);
        releaseLoad?.();
        options.onReady();
      } else if (type === "resize") {
        if (
          typeof payload.height !== "number" ||
          !Number.isFinite(payload.height) ||
          !Number.isSafeInteger(Math.ceil(payload.height)) ||
          payload.height < 1
        ) {
          reject();
          return;
        }
        options.onResize(Math.ceil(payload.height));
      } else if (type === "open_resource") {
        const ref = options.presentation.resource_refs?.find(
          (ref) => ref.id === payload.id && ref.type === payload.type,
        );
        if (!ref) {
          reject();
          return;
        }
        options.onResource?.(ref);
      } else if (type === "open_result") {
        if (
          typeof payload.ref !== "string" ||
          !options.presentation.result_refs?.includes(payload.ref)
        ) {
          reject();
          return;
        }
        options.onResult?.(payload.ref);
      } else if (type === "data_request") {
        if (
          !options.onRead ||
          !options.presentation.read_operations?.includes(
            String(payload.operation),
          )
        ) {
          reject();
          return;
        }
        const requestId = event.data.request_id;
        if (
          requests.has(requestId) ||
          requests.size >= 2048 ||
          activeReads >= 8
        ) {
          reject();
          return;
        }
        requests.add(requestId);
        activeReads++;
        void options
          .onRead({
            operation: payload.operation as TemplateReadOperation,
            parameters: payload.parameters as Record<string, unknown>,
          })
          .then(
            (result) => {
              if (destroyed || !port) return;
              const reply = envelope("data_response", { result }, requestId);
              if (messageBytes(reply) > TEMPLATE_MAX_BYTES) {
                port.postMessage(
                  envelope(
                    "data_response",
                    { error: "PRESENTATION_RESPONSE_TOO_LARGE" },
                    requestId,
                  ),
                );
                return;
              }
              port.postMessage(reply);
            },
            () => {
              if (!destroyed)
                port?.postMessage(
                  envelope(
                    "data_response",
                    { error: "PRESENTATION_READ_FAILED" },
                    requestId,
                  ),
                );
            },
          )
          .finally(() => {
            activeReads--;
          });
      } else if (type === "error") {
        options.onError("TEMPLATE_RENDER_FAILED");
        destroy();
      } else reject();
    };
    port.start();
    const hello = envelope("hello", { ...options.presentation, context });
    if (messageBytes(hello) > TEMPLATE_MAX_BYTES) {
      reject();
      return;
    }
    // allow-scripts alone makes this iframe opaque. The transferred port binds
    // the exact contentWindow; the runtime verifies its parent origin/source.
    iframe.contentWindow.postMessage(hello, "*", [channel.port2]);
  };
  function destroy() {
    if (destroyed) return;
    send("destroy", {});
    destroyed = true;
    window.clearTimeout(timeout);
    releaseLoad?.();
    iframe.removeEventListener("load", connect);
    port?.close();
    port = undefined;
    iframe.src = "about:blank";
  }
  iframe.setAttribute("sandbox", "allow-scripts");
  iframe.referrerPolicy = "no-referrer";
  target.pathname = "/";
  target.search = "";
  target.hash = "";
  target.searchParams.set("parent_origin", window.location.origin);
  releaseLoad = scheduleTemplateLoad((release) => {
    releaseLoad = release;
    iframe.addEventListener("load", connect, { once: true });
    timeout = window.setTimeout(() => {
      options.onError("TEMPLATE_LOAD_TIMEOUT");
      destroy();
    }, 15_000);
    iframe.src = target.toString();
  });
  return {
    destroy,
    notifyData() {
      if (version === "argus-template/v2") send("data_available", {});
    },
    setContext(value: TemplateContext) {
      context = value;
      send("context", value);
    },
  };
}
