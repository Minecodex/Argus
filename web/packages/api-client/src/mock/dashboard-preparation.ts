import type { DashboardSchemas } from "../dashboard";
import { mockConvertPanel } from "./dashboard-conversion";
import { mockGenerateDrilldowns } from "./dashboard-runtime";
import { copyDashboard, dashboardFailure } from "./dashboard-state";

export function prepareMockDraftSpec(
  original: DashboardSchemas["DashboardSpec"],
  preparation?: DashboardSchemas["DashboardDraftPreparation"],
): DashboardSchemas["DashboardSpec"] {
  const spec = copyDashboard(original);
  if (!preparation) return spec;
  if (
    (preparation.conversions?.length ?? 0) > 64 ||
    (preparation.drilldowns?.length ?? 0) > 64
  )
    dashboardFailure();
  const converted = new Set<string>();
  for (const conversion of preparation.conversions ?? []) {
    const index = spec.panels.findIndex(
      (panel) => panel.id === conversion.panel_id,
    );
    if (index < 0 || converted.has(conversion.panel_id)) dashboardFailure();
    converted.add(conversion.panel_id);
    const panel = spec.panels[index] ?? dashboardFailure();
    const result = mockConvertPanel({
      panel,
      mode: conversion.mode,
    });
    if (!result.converted) dashboardFailure();
    spec.panels[index] = result.panel;
  }
  const generated = new Set<string>();
  for (const drilldowns of preparation.drilldowns ?? []) {
    if (!drilldowns.panel_id || generated.has(drilldowns.panel_id))
      dashboardFailure();
    generated.add(drilldowns.panel_id);
    mockGenerateDrilldowns(spec, drilldowns.panel_id);
  }
  return spec;
}
