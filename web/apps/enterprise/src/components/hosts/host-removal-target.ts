import type { BastionScope, Connector, Host } from "@argus/api-client";

export type RemovalTarget = {
  type: "managed_host" | "bastion_scope";
  id: string;
  hostId: string;
  name: string;
  expectedVersion: number;
  platform: "linux" | "windows";
  address: string;
  port: number;
  installMethod: "manual" | "ssh";
  sshPath: "none" | "direct_executor" | "bastion_connector";
  bastionScopeId?: string;
  registeredConnectorId?: string;
  onboardingState: string;
  status: string;
  connectionStatus: string;
  existingOperationId?: string;
};

export function removalTargetForHost(host: Host): RemovalTarget {
  return {
    type: "managed_host",
    id: host.id,
    hostId: host.id,
    name: host.name,
    expectedVersion: host.resource_version,
    platform: host.platform,
    address: host.address,
    port: host.port || 22,
    installMethod: host.onboarding.install_method ?? "manual",
    sshPath: host.onboarding.ssh_path ?? "none",
    bastionScopeId: host.bastion_scope_id,
    registeredConnectorId: host.connector_id,
    onboardingState: host.onboarding.state,
    status: host.status,
    connectionStatus: host.connection_status,
    existingOperationId: host.removal_operation_id,
  };
}

export function removalTargetForBastion(
  scope: BastionScope,
  host: Host,
  connector?: Connector,
): RemovalTarget {
  const automatic = scope.onboarding_mode !== "command";
  return {
    type: "bastion_scope",
    id: scope.id,
    hostId: host.id,
    name: scope.name,
    expectedVersion: scope.resource_version,
    platform: "linux",
    address: host.address,
    port: host.port || 22,
    installMethod: automatic ? "ssh" : "manual",
    sshPath: automatic ? "direct_executor" : "none",
    registeredConnectorId: scope.active_connector_id,
    onboardingState: scope.onboarding.state,
    status: scope.status,
    connectionStatus: connector
      ? connector.status === "online"
        ? "online"
        : "offline"
      : host.connection_status,
    existingOperationId: scope.removal_operation_id,
  };
}
