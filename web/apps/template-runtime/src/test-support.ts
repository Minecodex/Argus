import * as renderers from "./renderers";
import fixtures from "./fixtures.generated.json";
import { flushSync } from "react-dom";
import { dashboardDefinition, clearViews } from "./views";

export function presentTool(
  tool: string,
  value?: Record<string, unknown>,
  locale = "zh-CN",
) {
  if (!window.matchMedia)
    window.matchMedia = (query) => ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener() {},
      removeEventListener() {},
      addListener() {},
      removeListener() {},
      dispatchEvent: () => false,
    });
  if (!globalThis.ResizeObserver)
    globalThis.ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
  clearViews();
  const binding = fixtures.bindings.find((row) => row.Tool === tool)!;
  const source = fixtures.assets[binding.Hash as keyof typeof fixtures.assets];
  const data = value ?? binding.Data;
  const normalized = { ...data };
  const payload = normalized.data as Record<string, unknown> | undefined;
  const page = normalized.page as Record<string, unknown> | undefined;
  const pageItems =
    normalized.items ??
    page?.items ??
    payload?.result ??
    payload?.rows ??
    (Array.isArray(payload) ? payload : []);
  document.documentElement.lang = locale;
  const parsed = document.createElement("template");
  parsed.innerHTML = source;
  const scripts = Array.from(parsed.content.querySelectorAll("script"));
  scripts.forEach((script) => script.remove());
  document.body.replaceChildren(parsed.content);
  const charts: Record<string, unknown>[] = [];
  const api = {
    ...renderers,
    dashboardDefinition: (...args: Parameters<typeof dashboardDefinition>) =>
      flushSync(() => dashboardDefinition(...args)),
    observation: (_root: HTMLElement, spec: Record<string, unknown>) =>
      charts.push(spec),
    onData: (callback: (data: unknown, context: unknown) => void) =>
      callback(normalized, { locale }),
    onDataChange: () => () => {},
    read: async (
      operation: string,
      parameters: Record<string, unknown> = {},
    ) => ({
      status: "ready",
      data:
        operation === "result.page"
          ? {
              items: Array.isArray(
                parameters.collection === "result"
                  ? (
                      normalized.result as Record<string, unknown>[] | undefined
                    )?.[Number(parameters.index)]?.[String(parameters.field)]
                  : pageItems,
              )
                ? (
                    (parameters.collection === "result"
                      ? (normalized.result as Record<string, unknown>[])[
                          Number(parameters.index)
                        ]![String(parameters.field)]
                      : pageItems) as unknown[]
                  ).slice(0, Number(parameters.limit ?? 25))
                : [],
              offset: 0,
              complete: true,
            }
          : normalized,
    }),
    openResource: () => {},
  };
  for (const script of scripts)
    new Function("ArgusTemplate", "ArgusView", script.textContent!)(
      api,
      (window as unknown as { ArgusView: unknown }).ArgusView,
    );
  return charts;
}

export { fixtures };
