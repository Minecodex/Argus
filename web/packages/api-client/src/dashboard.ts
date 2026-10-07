import type { components } from "./generated/dashboardapi";
import type { PendingActionPublic } from "./types";
export type DashboardSchemas = components["schemas"];
export type DashboardSpec = DashboardSchemas["DashboardSpec"];
export type DashboardPanel = DashboardSchemas["DashboardPanel"];
export type DashboardTarget = DashboardSchemas["DashboardTarget"];
export type DashboardVariable = DashboardSchemas["DashboardVariable"];
export type DashboardSelection = DashboardSchemas["DashboardSelection"];
export type DashboardDraft = DashboardSchemas["DashboardDraft"];
export type DashboardItem = DashboardSchemas["DashboardItem"];
export type DashboardRevision = DashboardSchemas["DashboardRevision"];
export type DashboardExecution = DashboardSchemas["DashboardExecution"];
export type DashboardFolder = DashboardSchemas["DashboardFolder"];
export type DashboardResourceType = "host" | "kubernetes_cluster";
export type DashboardBindingView = DashboardSchemas["DashboardBindingView"];
export interface DashboardDomains {
  dashboardQueries: {
    start(
      conversation: string,
      input: DashboardSchemas["DashboardQueryJobInput"],
      requestKey: string,
    ): Promise<DashboardSchemas["DashboardQueryJobView"]>;
    get(
      conversation: string,
      id: string,
    ): Promise<DashboardSchemas["DashboardQueryJobView"]>;
    cancel(
      conversation: string,
      id: string,
    ): Promise<DashboardSchemas["DashboardQueryJobView"]>;
    resume(
      conversation: string,
      id: string,
      version: number,
    ): Promise<DashboardSchemas["DashboardQueryJobView"]>;
  };
  dashboards: {
    convertPanel(
      input: DashboardSchemas["DashboardConvertPanelInput"],
    ): Promise<DashboardSchemas["DashboardConvertedPanel"]>;
    bindings(id: string): Promise<DashboardBindingView[]>;
    resourceBindings(
      type: DashboardResourceType,
      id: string,
    ): Promise<DashboardSchemas["ResourceDashboardBindings"]>;
    previewBinding(
      type: DashboardResourceType,
      id: string,
      input: DashboardSchemas["DashboardBindingInput"],
    ): Promise<PendingActionPublic>;
    list(): Promise<DashboardItem[]>;
    get(id: string): Promise<DashboardSchemas["DashboardDetail"]>;
    revisions(id: string): Promise<DashboardRevision[]>;
    drafts(): Promise<DashboardDraft[]>;
    draft(id: string): Promise<DashboardDraft>;
    createDraft(
      input: DashboardSchemas["DashboardDraftInput"],
    ): Promise<DashboardDraft>;
    saveDraft(
      id: string,
      input: DashboardSchemas["DashboardDraftInput"],
    ): Promise<DashboardDraft>;
    discardDraft(id: string, version: number): Promise<void>;
    rebase(
      id: string,
      input: DashboardSchemas["DashboardRebaseInput"],
    ): Promise<DashboardDraft>;
    validate(
      spec: DashboardSpec,
    ): Promise<DashboardSchemas["DashboardValidationReport"]>;
    sample(
      id: string,
      input: DashboardSchemas["DashboardDraftSampleInput"],
      signal?: AbortSignal,
    ): Promise<DashboardSchemas["DashboardDraftSample"]>;
    draftDrilldown(
      id: string,
      input: DashboardSchemas["DashboardDraftDrilldownInput"],
      signal?: AbortSignal,
    ): Promise<DashboardSchemas["DashboardDraftDrilldownExecution"]>;
    preview(id: string, version: number): Promise<PendingActionPublic>;
    execute(
      id: string,
      input: DashboardSchemas["DashboardExecutionInput"],
      signal?: AbortSignal,
    ): Promise<DashboardExecution>;
    catalog(
      input: DashboardSchemas["DashboardCatalogInput"],
      signal?: AbortSignal,
    ): Promise<DashboardSchemas["DashboardCatalogResult"]>;
    drilldown(
      id: string,
      input: DashboardSchemas["DashboardDrilldownInput"],
      signal?: AbortSignal,
    ): Promise<DashboardSchemas["DashboardDrilldownExecution"]>;
    generateDrilldowns(
      id: string,
      input: DashboardSchemas["DashboardGenerateDrilldownsInput"],
    ): Promise<DashboardSchemas["DashboardGeneratedDrilldowns"]>;
    folders(): Promise<DashboardFolder[]>;
    previewFolder(
      input: DashboardSchemas["DashboardLifecycleInput"],
    ): Promise<PendingActionPublic>;
    previewLifecycle(
      id: string,
      input: DashboardSchemas["DashboardLifecycleInput"],
    ): Promise<PendingActionPublic>;
  };
}

export function emptyDashboardSpec(): DashboardSpec {
  return {
    schema_version: "argus.telemetry_dashboard/v1",
    default_time_range: { kind: "relative", seconds: 3600 },
    default_refresh_seconds: 0,
    variables: [],
    panels: [],
    layout: { columns: 12, row_height: 8 },
  };
}
