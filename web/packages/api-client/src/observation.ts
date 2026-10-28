import type { components } from "./generated/telemetryapi";

export type ObservationScope = components["schemas"]["TelemetryQueryScope"];
export type ObservationResource = components["schemas"]["ObservationResource"];
export type ObservationResourceRef =
  components["schemas"]["ObservationResourceRef"];
export type ObservationResourcePage =
  components["schemas"]["ObservationResourcePage"];
export type ObservationCatalogInput = Omit<
  components["schemas"]["ObservationCatalogInput"],
  "observed_only"
> & { observed_only?: boolean };
export type ObservationCatalogResult =
  components["schemas"]["ObservationCatalogResult"];
export type ObservationMetric = components["schemas"]["ObservationMetric"];
export type ObservationRecipe = components["schemas"]["ObservationRecipe"];
export type ObservationSource = components["schemas"]["ObservationSource"];
export type ObservationSignal = ObservationCatalogInput["signal"];
export type ObservationResourceQuery = {
  kind: ObservationResourceRef["type"];
  search?: string;
  cursor?: string;
  limit?: number;
};
export interface ObservationDiscovery {
  resources(
    query: ObservationResourceQuery,
    signal?: AbortSignal,
  ): Promise<ObservationResourcePage>;
  catalog(
    query: ObservationCatalogInput,
    signal?: AbortSignal,
  ): Promise<ObservationCatalogResult>;
}
