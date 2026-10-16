import { createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import {
  LocaleProvider,
  ThemeProvider,
  ResultTable,
  ResultFilters,
  ResultPager,
  type ResultPagerProps,
  RecordFields,
  DashboardDefinitionView,
  type ResultColumn,
  type ResultRow,
  type ResultFilterState,
} from "@argus/ui/result";

const roots = new Map<HTMLElement, Root>();
function mount(target: HTMLElement, children: ReactNode) {
  const root = roots.get(target) ?? createRoot(target);
  roots.set(target, root);
  root.render(
    createElement(LocaleProvider, {
      locale: document.documentElement.lang === "en-US" ? "en-US" : "zh-CN",
      persist: false,
      children: createElement(ThemeProvider, {
        theme:
          document.documentElement.dataset.theme === "dark" ? "dark" : "light",
        persist: false,
        children,
      }),
    }),
  );
}
export function recordTable(
  target: HTMLElement,
  props: {
    rows: ResultRow[];
    columns: ResultColumn[];
    detail?: (target: HTMLElement, row: ResultRow) => void;
  },
) {
  mount(target, createElement(ResultTable, props));
}
export function resultFilters(
  target: HTMLElement,
  props: {
    value: ResultFilterState;
    facets: Record<string, string[]>;
    onChange(value: ResultFilterState): void;
    busy?: boolean;
  },
) {
  mount(target, createElement(ResultFilters, props));
}
export function clearViews(container?: HTMLElement) {
  for (const [node, root] of roots)
    if (
      !container ||
      node === container ||
      container.contains(node) ||
      !node.isConnected
    ) {
      root.unmount();
      roots.delete(node);
    }
}
export function resultPager(target: HTMLElement, props: ResultPagerProps) {
  mount(target, createElement(ResultPager, props));
}
export function dashboardDefinition(
  target: HTMLElement,
  props: {
    definition: Record<string, unknown>;
    metadata?: Record<string, unknown>;
  },
) {
  mount(target, createElement(DashboardDefinitionView, props));
}
export function recordFields(
  target: HTMLElement,
  props: { fields: [string, unknown][] },
) {
  mount(target, createElement(RecordFields, props));
}
