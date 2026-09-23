import {
  TEMPLATE_PROTOCOL,
  TEMPLATE_MAX_BYTES,
  isTemplateMessage,
  messageBytes,
  type TemplateContext,
  type TemplatePresentation,
  type TemplateResource,
} from "./protocol";

export type TemplateHostOptions = {
  origin: string;
  presentation: TemplatePresentation;
  context: TemplateContext;
  onReady(): void;
  onError(reason: string): void;
  onResize(height: number): void;
  onResource?(ref: TemplateResource): void;
  onResult?(ref: string): void;
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
  let outgoing = 0,
    incoming = 0,
    destroyed = false;
  let port: MessagePort | undefined;
  let context = options.context;
  const envelope = (type: string, payload: Record<string, unknown>) => ({
    version: TEMPLATE_PROTOCOL,
    nonce,
    sequence: ++outgoing,
    request_id: crypto.randomUUID(),
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
  const timeout = window.setTimeout(() => {
    options.onError("TEMPLATE_LOAD_TIMEOUT");
    destroy();
  }, 15_000);
  const connect = () => {
    if (destroyed || port || !iframe.contentWindow) return;
    const channel = new MessageChannel();
    port = channel.port1;
    port.onmessage = (event: MessageEvent<unknown>) => {
      if (destroyed) return;
      if (
        !isTemplateMessage(event.data) ||
        event.data.nonce !== nonce ||
        event.data.sequence <= incoming
      ) {
        reject();
        return;
      }
      const { type, payload, sequence } = event.data;
      incoming = sequence;
      if (type === "ready") {
        window.clearTimeout(timeout);
        options.onReady();
      } else if (type === "resize") {
        if (
          typeof payload.height !== "number" ||
          !Number.isFinite(payload.height) ||
          payload.height < 1
        ) {
          reject();
          return;
        }
        options.onResize(Math.min(Math.ceil(payload.height), 2000));
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
    iframe.removeEventListener("load", connect);
    port?.close();
    port = undefined;
    iframe.src = "about:blank";
  }
  iframe.setAttribute("sandbox", "allow-scripts");
  iframe.referrerPolicy = "no-referrer";
  iframe.addEventListener("load", connect, { once: true });
  target.pathname = "/";
  target.search = "";
  target.hash = "";
  target.searchParams.set("parent_origin", window.location.origin);
  iframe.src = target.toString();
  return {
    destroy,
    setContext(value: TemplateContext) {
      context = value;
      send("context", value);
    },
  };
}
