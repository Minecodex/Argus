import type { components } from "./generated/monitoringapi";
import type { PendingActionPublic } from "./types";
import {
  collectorVersion,
  collectorDistributionVersion,
} from "./generated/collector-registry";

export type MonitoringSchemas = components["schemas"];
export type MonitoringApplication = MonitoringSchemas["MonitoringApplication"];
export type MonitoringApplicationInput =
  MonitoringSchemas["MonitoringApplicationInput"];
export type MonitoringService =
  MonitoringSchemas["MonitoringApplicationService"];
export type MonitoringMutation = MonitoringSchemas["MonitoringMutation"];
export type MonitoringCollection = MonitoringSchemas["MonitoringCollection"];
export type MonitoringIntegration = MonitoringSchemas["MonitoringIntegration"];
export type MonitoringTarget = MonitoringSchemas["MonitoringTarget"];
export type MonitoringListInput = {
  environment?: string;
  search?: string;
  cursor?: string;
  limit?: number;
  status?: "active" | "archived";
  application_ids?: string[];
  integration?: string;
  desired_enabled?: boolean;
  apply_status?: MonitoringCollection["apply_status"];
  health?: MonitoringCollection["health"];
  freshness?: MonitoringCollection["freshness"];
};

// Match the server's locked distribution admission, including its version.
// A component name alone does not prove that an older receiver collects data.
export function monitoringCollectorCompatible(
  item: MonitoringSchemas["MonitoringCollectorChoice"],
  integration: Pick<MonitoringIntegration, "component" | "platforms">,
  requiredComponents: readonly string[] = [],
  resourceType?: "host" | "kubernetes_cluster",
): boolean {
  return (
    (!resourceType || item.resource.kind === resourceType) &&
    item.support_status === "supported" &&
    item.collector_version === collectorVersion &&
    (!requiredComponents.includes("argus_application_identity") ||
      item.distribution_version === collectorDistributionVersion) &&
    !["uninstalled", "uninstalling"].includes(item.status) &&
    item.components.includes(integration.component ?? "") &&
    requiredComponents.every((component) =>
      item.components.includes(component),
    ) &&
    integration.platforms.includes(item.platform)
  );
}
export interface MonitoringDomains {
  monitoring: {
    ingestConnection(
      endpointId: string,
    ): Promise<MonitoringSchemas["MonitoringIngestConnection"]>;
    builtins(target: {
      kind: "host" | "kubernetes_cluster";
      id: string;
    }): Promise<MonitoringSchemas["MonitoringBuiltin"][]>;
    previewBuiltin(
      input: MonitoringSchemas["MonitoringBuiltinInput"],
    ): Promise<PendingActionPublic>;
    ingestEndpoints(
      applicationId: string,
      input?: MonitoringListInput,
    ): Promise<MonitoringSchemas["MonitoringIngestEndpointPage"]>;
    ingestKeys(
      endpointId: string,
      input?: MonitoringListInput,
    ): Promise<MonitoringSchemas["MonitoringIngestKeyPage"]>;
    revealIngestKey(
      keyId: string,
    ): Promise<MonitoringSchemas["MonitoringIngestKeyReveal"]>;
    collectors(
      input?: MonitoringListInput,
    ): Promise<MonitoringSchemas["MonitoringCollectorChoicePage"]>;
    integrations(): Promise<MonitoringIntegration[]>;
    collections(
      input?: MonitoringListInput,
    ): Promise<MonitoringSchemas["MonitoringCollectionPage"]>;
    collection(id: string): Promise<MonitoringCollection>;
    targets(
      input?: MonitoringListInput,
    ): Promise<MonitoringSchemas["MonitoringTargetPage"]>;
    applications(
      input?: MonitoringListInput,
    ): Promise<MonitoringSchemas["MonitoringApplicationPage"]>;
    application(id: string): Promise<MonitoringApplication>;
    services(
      input?: MonitoringListInput,
    ): Promise<MonitoringSchemas["MonitoringServicePage"]>;
    preview(input: MonitoringMutation): Promise<PendingActionPublic>;
  };
}
