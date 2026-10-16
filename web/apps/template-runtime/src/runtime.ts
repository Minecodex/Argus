import { baseStyle } from "./runtime-style";
import {
  TEMPLATE_MAX_BYTES,
  isTemplateMessage,
  messageBytes,
  templateHash,
  type TemplateContext,
  type TemplateMessage,
} from "@argus/ui/template";
import * as renderers from "./renderers";

export function trustedHello(
  event: MessageEvent<unknown>,
  parent: Window,
  expectedOrigin: string | null,
): event is MessageEvent<TemplateMessage> {
  if (
    !expectedOrigin ||
    event.source !== parent ||
    event.origin !== expectedOrigin ||
    event.ports.length !== 1
  )
    return false;
  try {
    if (new URL(expectedOrigin).origin !== expectedOrigin) return false;
  } catch {
    return false;
  }
  return (
    isTemplateMessage(event.data) &&
    event.data.type === "hello" &&
    event.data.sequence === 1
  );
}
export function csp(nonce: string) {
  return `default-src 'none'; script-src 'nonce-${nonce}'; style-src 'unsafe-inline'; connect-src 'none'; img-src 'none'; font-src 'none'; object-src 'none'; worker-src 'none'; frame-src 'none'; base-uri 'none'; form-action 'none'`;
}

export async function startRuntime(
  hello: TemplateMessage,
  port: MessagePort,
): Promise<void> {
  const payload = hello.payload;
  const source = payload.template_source;
  if (
    typeof source !== "string" ||
    new TextEncoder().encode(source).length > 256 * 1024 ||
    typeof payload.template_hash !== "string" ||
    (await templateHash(source)) !== payload.template_hash
  )
    throw new Error("invalid template hash");
  let context = payload.context as TemplateContext;
  if (
    !context ||
    !["zh-CN", "en-US"].includes(context.locale) ||
    !["light", "dark"].includes(context.color_scheme) ||
    !context.tokens
  )
    throw new Error("invalid context");
  const data = payload.detail_data;
  // Load only version-locked, bundled primitives before installing connect-none
  // CSP. Tool source executes after every dependency has finished loading.
  const { observation, disposeObservations, componentStyles } =
    await import("./observation");
  const {
    recordTable,
    resultFilters,
    resultPager,
    recordFields,
    dashboardDefinition,
    clearViews,
  } = await import("./views");
  if (!data || typeof data !== "object" || Array.isArray(data))
    throw new Error("invalid detail data");
  let outgoing = 0,
    incoming = hello.sequence,
    destroyed = false;
  const waiting = new Map<
    string,
    {
      resolve: (result: unknown) => void;
      reject: (error: Error) => void;
      timer: ReturnType<typeof setTimeout>;
    }
  >();
  const changes = new Set<() => void>();
  let listener:
    ((value: unknown, context: TemplateContext) => void) | undefined;
  const send = (
    type: string,
    payload: Record<string, unknown>,
    requestId?: string,
  ) => {
    if (destroyed) return;
    const message = {
      version: hello.version,
      nonce: hello.nonce,
      sequence: ++outgoing,
      request_id: requestId ?? crypto.randomUUID(),
      type,
      payload,
    };
    if (messageBytes(message) <= TEMPLATE_MAX_BYTES) port.postMessage(message);
  };
  const fail = () => send("error", { code: "TEMPLATE_RENDER_FAILED" });
  const applyContext = () => {
    document.documentElement.lang = context.locale;
    document.documentElement.style.colorScheme = context.color_scheme;
    document.documentElement.dataset.theme = context.color_scheme;
    document.documentElement.dataset.templateLayout =
      context.layout === "document" ? "document" : "preview";
    for (const [key, value] of Object.entries(context.tokens)) {
      if (
        /^--[a-z0-9-]+$/.test(key) &&
        typeof value === "string" &&
        value.length < 1024 &&
        !/url\(|expression\(|[{}<>]/i.test(value)
      )
        document.documentElement.style.setProperty(key, value);
    }
  };
  const render = () => {
    try {
      applyContext();
      listener?.(data, context);
    } catch {
      fail();
    }
  };
  const api = Object.freeze({
    ...renderers,
    observation,
    recordTable,
    resultFilters,
    resultPager,
    recordFields,
    dashboardDefinition,
    clearViews,
    clearObservations: () => {
      clearViews();
      disposeObservations();
    },
    onData(value: typeof listener) {
      listener = value;
      render();
    },
    onDataChange(callback: () => void) {
      changes.add(callback);
      return () => changes.delete(callback);
    },
    read(operation: string, parameters: Record<string, unknown> = {}) {
      if (
        destroyed ||
        hello.version !== "argus-template/v2" ||
        !Array.isArray(payload.read_operations) ||
        !payload.read_operations.includes(operation)
      )
        return Promise.reject(new Error("PRESENTATION_READ_FORBIDDEN"));
      return new Promise((resolve, reject) => {
        const id = crypto.randomUUID();
        const timer = setTimeout(() => {
          waiting.delete(id);
          reject(new Error("PRESENTATION_READ_TIMEOUT"));
        }, 15_000);
        waiting.set(id, { resolve, reject, timer });
        send("data_request", { operation, parameters }, id);
      });
    },
    openResource(type: string, id: string) {
      send("open_resource", { type, id });
    },
    openResult(ref: string) {
      send("open_result", { ref });
    },
  });
  Object.defineProperty(window, "ArgusTemplate", {
    value: api,
    configurable: false,
    writable: false,
  });
  const policy = document.createElement("meta");
  policy.httpEquiv = "Content-Security-Policy";
  policy.content = csp(hello.nonce);
  document.head.append(policy);
  const styles = document.createElement("style");
  styles.textContent = componentStyles + "\n" + baseStyle;
  document.head.append(styles);
  const parsed = new DOMParser().parseFromString(source, "text/html");
  if (
    parsed.querySelector("iframe,object,embed,form,base,meta,link,script[src]")
  )
    throw new Error("unsupported template capability");
  for (const element of parsed.querySelectorAll("*"))
    for (const attribute of element.attributes)
      if (/^on/i.test(attribute.name))
        throw new Error("inline event attributes are forbidden");
  const scripts = Array.from(parsed.querySelectorAll("script"));
  scripts.forEach((script) => script.remove());
  const root = document.getElementById("argus-template-root")!;
  // DOMParser places leading styles in <head>. Preserve those immutable styles
  // as well as body styles; otherwise shared template geometry silently drops.
  root.replaceChildren(
    ...Array.from(parsed.head.querySelectorAll("style")),
    ...Array.from(parsed.body.childNodes),
  );
  applyContext();
  window.addEventListener("error", fail);
  window.addEventListener("unhandledrejection", fail);
  for (const script of scripts) {
    const active = document.createElement("script");
    active.nonce = hello.nonce;
    active.textContent = script.textContent;
    root.append(active);
    active.remove();
  }
  let pendingFrame = 0;
  const observer = new ResizeObserver(() => {
    cancelAnimationFrame(pendingFrame);
    pendingFrame = requestAnimationFrame(() =>
      send("resize", { height: intrinsicTemplateHeight(root) }),
    );
  });
  observer.observe(root);
  port.onmessage = (event: MessageEvent<unknown>) => {
    if (destroyed) return;
    if (
      !isTemplateMessage(event.data) ||
      event.data.nonce !== hello.nonce ||
      event.data.version !== hello.version ||
      event.data.sequence <= incoming
    ) {
      fail();
      return;
    }
    incoming = event.data.sequence;
    if (event.data.type === "context") {
      context = event.data.payload as TemplateContext;
      render();
    } else if (event.data.type === "destroy") {
      observer.disconnect();
      cancelAnimationFrame(pendingFrame);
      destroyed = true;
      disposeObservations();
      clearViews();
      for (const request of waiting.values()) {
        clearTimeout(request.timer);
        request.reject(new Error("PRESENTATION_DESTROYED"));
      }
      waiting.clear();
      changes.clear();
      listener = undefined;
      port.close();
      root.replaceChildren();
    } else if (event.data.type === "data_response") {
      const request = waiting.get(event.data.request_id);
      if (!request) {
        fail();
        return;
      }
      clearTimeout(request.timer);
      waiting.delete(event.data.request_id);
      if (event.data.payload.error)
        request.reject(new Error(String(event.data.payload.error)));
      else request.resolve(event.data.payload.result);
    } else if (event.data.type === "data_available") {
      for (const callback of changes) callback();
    } else fail();
  };
  port.start();
  send("ready", {});
}

export function intrinsicTemplateHeight(root: HTMLElement): number {
  const body = root.ownerDocument.body;
  const style = getComputedStyle(body);
  return Math.ceil(
    Math.max(root.scrollHeight, root.getBoundingClientRect().height) +
      (parseFloat(style.paddingTop) || 0) +
      (parseFloat(style.paddingBottom) || 0),
  );
}
