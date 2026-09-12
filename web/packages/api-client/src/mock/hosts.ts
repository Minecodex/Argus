import type { ArgusApiClient } from "../client";
import type {
  Host,
  HostPage,
  HostRemovalOperation,
} from "../generated/contracts";
import type { MockHost } from "./resource-models";
import type { MockContext } from "./context";
import type { HostConnectionTests } from "./host-connection-tests";
import { nextId, settleCollectors } from "./store";
import { ApiError } from "../transport/errors";
import {
  resourceNameAvailable,
  requireAvailableResourceName,
} from "./name-availability";

function hostContract(value: MockHost): Host {
  const contract: Host = {
    id: value.id,
    enterprise_id: value.enterpriseId,
    name: value.name,
    hostname: value.hostname || undefined,
    address: value.address,
    port: value.port,
    platform: value.platform,
    role: value.role,
    control_path: value.controlPath,
    bastion_scope_id: value.bastionScopeId,
    connector_id: value.connectorId,
    runtime: {
      platform: value.platform,
      openssh_status: "available",
      rdp_status:
        value.platform === "windows"
          ? value.rdpEnabled
            ? "enabled"
            : "disabled"
          : "unavailable",
      rdp_nla_enabled: Boolean(value.rdpEnabled),
      rdp_firewall_enabled: Boolean(value.rdpEnabled),
      rdp_service_running: value.platform === "windows",
      observed_at: value.updatedAt,
    },
    environment: value.environment,
    labels: value.labels,
    labels_version: 1,
    resource_version: value.resourceVersion ?? 1,
    connection_status: value.connectionStatus,
    architecture: value.architecture,
    last_seen_at: value.lastSeenAt,
    status: value.status ?? "active",
    removal_generation: value.removalGeneration ?? 0,
    local_cleanup: value.localCleanup ?? "verified",
    onboarding: {
      install_method: value.installMethod ?? "manual",
      ssh_path: value.installSSHPath ?? "none",
      state:
        value.onboardingState ??
        (value.connectionStatus === "onboarding" ? "installing" : "registered"),
      execution_id: value.onboardingExecutionId,
      operation_id: value.onboardingOperationId,
      error_code: value.onboardingErrorCode,
      updated_at: value.updatedAt,
    },
    created_at: value.createdAt,
    updated_at: value.updatedAt,
  };
  return Object.assign(contract, {
    collectorStatus: value.collectorStatus,
    telemetryRoute: value.telemetryRoute,
  });
}

function hostPage(items: MockHost[], limit?: number): HostPage {
  return {
    items: items.slice(0, limit).map(hostContract),
    page: {
      next_cursor: null,
      has_more: limit !== undefined && items.length > limit,
      partial: { partial: false, reasons: [] },
    },
  };
}

/** Hosts, remote access sessions and host collector management. */
export function createHostsDomain(
  ctx: MockContext,
  secrets: ArgusApiClient["secrets"],
  connectionTests: HostConnectionTests,
): ArgusApiClient["hosts"] {
  const { db } = ctx;
  const removals = new Map<string, HostRemovalOperation>();

  return {
    async checkNameAvailability(name) {
      await ctx.pause();
      return { available: resourceNameAvailable(db, ctx.enterpriseId(), name) };
    },
    async list(filter) {
      await ctx.pause();
      let items = db.hosts.filter(
        (entry) => entry.enterpriseId === ctx.enterpriseId(),
      );
      if (filter?.query) {
        const q = filter.query.toLowerCase();
        items = items.filter(
          (entry) =>
            entry.name.toLowerCase().includes(q) || entry.address.includes(q),
        );
      }
      if (filter?.control_path) {
        items = items.filter(
          (entry) => entry.controlPath === filter.control_path,
        );
      }
      if (filter?.bastion_scope_id) {
        items = items.filter(
          (entry) => entry.bastionScopeId === filter.bastion_scope_id,
        );
      }
      if (filter?.labels) {
        items = items.filter((entry) =>
          Object.entries(filter.labels ?? {}).every(([key, values]) =>
            values.includes(entry.labels[key] ?? ""),
          ),
        );
      }
      return hostPage(items, filter?.limit);
    },
    async get(id) {
      await ctx.pause();
      return hostContract(
        ctx.mustFind(db.hosts, (entry) => entry.id === id, "host"),
      );
    },
    async createConnectionTest(input) {
      return connectionTests.create(input);
    },
    async getConnectionTest(id) {
      return connectionTests.get(id);
    },
    async previewCreateResource(input) {
      await ctx.pause();
      const name = requireAvailableResourceName(
        db,
        ctx.enterpriseId(),
        input.name,
      );
      if (input.install_method === "ssh") {
        connectionTests.requireOnboarding({
          ...input,
          ssh_path:
            input.ssh_path === "bastion_connector"
              ? "bastion_connector"
              : "direct_executor",
          onboarding_control_path: input.control_path,
        });
      }
      return ctx.createPendingAction({
        tool: "host.create",
        title: `新增主机 ${input.name}`,
        input_data: { ...input, name },
      });
    },
    async previewUpdateResource(id, input) {
      await ctx.pause();
      const host = ctx.mustFind(db.hosts, (entry) => entry.id === id, "host");
      return ctx.createPendingAction({
        tool: "host.update",
        input_data: { id, name: host.name, ...input },
      });
    },
    async previewDeleteResource(id, expectedVersion) {
      await ctx.pause();
      const host = ctx.mustFind(db.hosts, (entry) => entry.id === id, "host");
      const currentConnector = db.connectors.find(
        (connector) =>
          connector.id === host.connectorId &&
          connector.status !== "revoked" &&
          connector.status !== "uninstalled",
      );
      if ((host.resourceVersion ?? 1) !== expectedVersion) {
        throw new Error("RESOURCE_VERSION_CONFLICT");
      }
      if (host.status !== "uninstalled" && currentConnector) {
        throw new Error("HOST_DELETE_REQUIRES_UNINSTALL");
      }
      const incompleteInstall = host.status !== "uninstalled";
      return ctx.createPendingAction({
        tool: "host.delete",
        risk: incompleteInstall ? "write" : undefined,
        input_data: {
          id,
          name: host.name,
          expected_version: expectedVersion,
          expected_connector_id: currentConnector?.id ?? "",
          delete_incomplete_install: incompleteInstall,
        },
      });
    },
    async getRemovalConnectionDefaults(input) {
      await ctx.pause();
      const scope =
        input.target_type === "bastion_scope"
          ? db.bastionScopes.find(
              (item) =>
                item.id === input.target_id &&
                item.enterpriseId === ctx.enterpriseId(),
            )
          : undefined;
      const host = db.hosts.find(
        (item) =>
          item.id === (scope?.connectorHostId ?? input.target_id) &&
          item.enterpriseId === ctx.enterpriseId(),
      );
      if (!host) throw new Error("HOST_REMOVAL_INVALID_TARGET");
      const connectorId = scope?.activeConnectorId ?? host.connectorId;
      if (
        !connectorId ||
        !db.connectors.some((connector) => connector.id === connectorId)
      ) {
        throw new ApiError(
          {
            code: "HOST_REMOVAL_NOT_INSTALLED",
            message_key: "errors.host_removal.not_installed",
            request_id: "mock-host-removal-not-installed",
            retryable: false,
          },
          409,
        );
      }
      const credential = (await secrets.listCredentials()).find(
        (item) =>
          item.id === host.installCredentialId &&
          item.status === "active" &&
          item.protocol === "ssh",
      );
      return {
        username: host.installUsername ?? "",
        status: credential ? "available" : "credential_unavailable",
        credential_id: credential?.id,
        credential_name: credential?.name,
      };
    },
    async previewRemoval(input) {
      await ctx.pause();
      const scope =
        input.target_type === "bastion_scope"
          ? db.bastionScopes.find((entry) => entry.id === input.target_id)
          : undefined;
      const dependencies = (scope?.memberHostIds ?? []).flatMap((hostId) => {
        const member = db.hosts.find((entry) => entry.id === hostId);
        return member
          ? [
              {
                type: "member_host",
                id: member.id,
                name: member.name,
                reason: member.controlPath,
              },
            ]
          : [];
      });
      const id = nextId(db, "remove");
      const now = ctx.nowIso();
      removals.set(id, {
        id,
        target_type: input.target_type,
        target_id: input.target_id,
        connector_id:
          db.hosts.find((host) => host.id === input.target_id)?.connectorId ??
          nextId(db, "connector"),
        mode: input.mode,
        delivery_method: input.mode === "forget" ? "server_only" : "manual",
        ssh_path: "none",
        target_platform: "linux_amd64",
        status:
          input.mode === "forget" ? "succeeded" : "awaiting_manual_execution",
        stage:
          input.mode === "forget" ? "completed" : "awaiting_manual_execution",
        attempt: 0,
        max_attempts: 10,
        local_cleanup: input.mode === "forget" ? "unknown" : "pending",
        events: [],
        expires_at: new Date(Date.now() + 30 * 60_000).toISOString(),
        created_at: now,
        updated_at: now,
      });
      return ctx.createPendingAction({
        tool: `host.removal.${input.mode}`,
        input_data: { ...input, operation_id: id, dependencies },
      });
    },
    async previewRetryResource(id, input) {
      await ctx.pause();
      connectionTests.requireOnboarding({
        ...input,
        ssh_path:
          input.ssh_path === "bastion_connector"
            ? "bastion_connector"
            : "direct_executor",
        onboarding_control_path: input.control_path,
      });
      return ctx.createPendingAction({
        tool: "host.onboarding.retry",
        title: "Retry host installation",
        input_data: { ...input, host_id: id },
      });
    },
    async getOnboardingOperation(id) {
      await ctx.pause();
      const host = db.hosts.find((item) => item.onboardingOperationId === id);
      if (!host) throw new Error("onboarding operation not found");
      const failed = host.onboardingState === "install_failed";
      const status = failed
        ? ("failed" as const)
        : host.connectionStatus === "online"
          ? ("succeeded" as const)
          : ("running" as const);
      const stage = failed
        ? ("transferring" as const)
        : status === "succeeded"
          ? ("completed" as const)
          : ("waiting_online" as const);
      return {
        id,
        host_id: host.id,
        connector_id:
          host.connectorId ?? "00000000-0000-0000-0000-000000000001",
        install_method: "ssh",
        ssh_path: host.bastionScopeId ? "bastion_connector" : "direct_executor",
        target_platform:
          host.platform === "windows"
            ? "windows_amd64"
            : host.architecture === "arm64"
              ? "linux_arm64"
              : "linux_amd64",
        control_path: host.controlPath,
        bastion_scope_id: host.bastionScopeId,
        status,
        stage,
        attempts: failed ? 3 : 1,
        max_attempts: 3,
        error_code: host.onboardingErrorCode,
        expires_at: host.updatedAt,
        created_at: host.createdAt,
        updated_at: host.updatedAt,
        events: [
          {
            id: `${id}-event`,
            stage,
            status: failed ? "failed" : "started",
            error_code: host.onboardingErrorCode,
            occurred_at: host.updatedAt,
          },
        ],
      };
    },
    async getRemovalOperation(id) {
      await ctx.pause();
      const value = removals.get(id);
      if (!value) throw new Error("removal operation not found");
      return value;
    },
    async retryRemovalOperation(id) {
      const value = await this.getRemovalOperation(id);
      value.status =
        value.delivery_method === "manual"
          ? "awaiting_manual_execution"
          : "queued";
      value.updated_at = ctx.nowIso();
      return value;
    },
    async regenerateRemovalCommand(id) {
      const value = await this.getRemovalOperation(id);
      return {
        operation_id: value.id,
        platform: value.target_platform.startsWith("windows_")
          ? "windows"
          : "linux",
        shell: value.target_platform.startsWith("windows_")
          ? "powershell"
          : "posix_sh",
        privilege: "system",
        command: value.target_platform.startsWith("windows_")
          ? "# mock PowerShell removal command"
          : "# mock POSIX removal command",
        expires_at: new Date(Date.now() + 15 * 60_000).toISOString(),
      };
    },
    async previewEnableWindowsRDP(id, expectedVersion) {
      await ctx.pause();
      const host = ctx.mustFind(db.hosts, (entry) => entry.id === id, "host");
      return ctx.createPendingAction({
        tool: "host.windows_rdp.enable",
        input_data: {
          host_id: id,
          name: host.name,
          expected_version: expectedVersion,
        },
      });
    },
    async getCollector(hostId) {
      await ctx.pause();
      settleCollectors(db);
      ctx.save();
      const found = db.collectors.find(
        (entry) =>
          entry.resource_type === "host" && entry.resource_id === hostId,
      );
      // 返回克隆:installing→converged 是原地变更,同引用会阻止轮询方重渲染。
      return found
        ? { ...found, route: found.route && { ...found.route } }
        : null;
    },
    async previewCollectorAction(hostId, action, input) {
      await ctx.pause();
      const host = ctx.mustFind(
        db.hosts,
        (entry) => entry.id === hostId,
        "host",
      );
      let gatewayCollectorId = input.gateway_collector_id;
      let bootstrapGatewayHostId: string | undefined;
      if (
        action === "install" &&
        input.route_kind === "bastion_gateway" &&
        !gatewayCollectorId
      ) {
        const scope = db.bastionScopes.find(
          (entry) =>
            entry.id === host.bastionScopeId &&
            entry.enterpriseId === host.enterpriseId,
        );
        const gatewayHostId = scope?.connectorHostId;
        if (!gatewayHostId) throw new Error("TELEMETRY_ROUTE_INVALID");
        const currentGateway = db.collectors.find(
          (entry) =>
            entry.enterprise_id === host.enterpriseId &&
            entry.resource_type === "host" &&
            entry.resource_id === gatewayHostId &&
            entry.role === "edge_gateway" &&
            entry.status !== "uninstalled",
        );
        gatewayCollectorId = currentGateway?.id ?? nextId(db, "col");
        if (!currentGateway) bootstrapGatewayHostId = gatewayHostId;
      }
      return ctx.createPendingAction({
        tool: `telemetry.host.${action}`,
        title: `${action} OTLP 收集器 · ${host.name}`,
        input_data: {
          host_id: hostId,
          distribution_version_id: input.distribution_version_id,
          profile_ids: input.profile_ids,
          route_kind: input.route_kind,
          transport: input.transport,
          loopback_port: input.loopback_port,
          gateway_collector_id: gatewayCollectorId,
          ...(bootstrapGatewayHostId
            ? {
                bootstrap_gateway_host_id: bootstrapGatewayHostId,
                sequence: [
                  {
                    order: 1,
                    operation: "enable_bastion_edge_gateway",
                    resource_id: bootstrapGatewayHostId,
                  },
                  {
                    order: 2,
                    operation: "install_leaf_collector",
                    resource_id: hostId,
                  },
                ],
              }
            : {}),
          expected_version: input.expected_version,
        },
      });
    },
    async previewCollectorInstall(hostId, input) {
      return this.previewCollectorAction(hostId, "install", input);
    },
  };
}
