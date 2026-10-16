import { createRoot, type Root } from "react-dom/client";
import { createElement } from "react";
import {
  LocaleProvider,
  ThemeProvider,
  ObservationPanel,
} from "@argus/ui/observation";
import foundationStyles from "@argus/ui/template-foundation.css?inline";
import sharedStyles from "@argus/ui/template-styles.css?inline";
export const componentStyles = foundationStyles + "\n" + sharedStyles;
import type { ObservationTarget } from "@argus/ui/observation";

const roots = new Map<HTMLElement, Root>();
export function observation(root: HTMLElement, spec: Record<string, unknown>) {
  const range =
    spec.range && typeof spec.range === "object"
      ? (spec.range as Record<string, unknown>)
      : undefined;
  const current = roots.get(root) ?? createRoot(root);
  roots.set(root, current);
  current.render(
    createElement(LocaleProvider, {
      locale: document.documentElement.lang === "en-US" ? "en-US" : "zh-CN",
      persist: false,
      children: createElement(ThemeProvider, {
        theme:
          document.documentElement.dataset.theme === "light" ? "light" : "dark",
        persist: false,
        children: createElement(ObservationPanel, {
          title: String(spec.title ?? spec.name ?? ""),
          type: String(spec.type ?? "timeseries"),
          signal: String(spec.signal ?? "metrics"),
          targets: (Array.isArray(spec.targets)
            ? spec.targets
            : []) as ObservationTarget[],
          unit: typeof spec.unit === "string" ? spec.unit : "",
          decimals: typeof spec.decimals === "number" ? spec.decimals : 2,
          legend: spec.legend !== false,
          display:
            spec.display &&
            typeof spec.display === "object" &&
            !Array.isArray(spec.display)
              ? (spec.display as never)
              : undefined,
          thresholds: (spec.thresholds ?? []) as never,
          range:
            typeof range?.from === "string" && typeof range.to === "string"
              ? { from: range.from, to: range.to }
              : undefined,
        }),
      }),
    }),
  );
}
export function disposeObservations() {
  for (const root of roots.values()) root.unmount();
  roots.clear();
}
