import { describe, expect, it } from "vitest";

import type { BastionScope, Host } from "@argus/api-client";

import {
  removalTargetForBastion,
  removalTargetForHost,
} from "./host-removal-target";

const timestamp = "2026-09-11T00:00:00Z";

function host(overrides: Partial<Host> = {}): Host {
  return {
    id: "host-1",
    enterprise_id: "enterprise-1",
    name: "host-1",
    address: "192.0.2.10",
    port: 22,
    platform: "linux",
    role: "managed_host",
    control_path: "direct",
    environment: "development",
    labels: {},
    labels_version: 1,
    resource_version: 3,
    connection_status: "online",
    status: "active",
    onboarding: {
      state: "registered",
      install_method: "ssh",
      ssh_path: "direct_executor",
      updated_at: timestamp,
    },
    created_at: timestamp,
    updated_at: timestamp,
    connector_id: "connector-host-1",
    ...overrides,
  };
}

describe("removal target builders", () => {
  it.each(["direct", "executor_tunnel"] as const)(
    "keeps registered direct Host metadata for %s control",
    (controlPath) => {
      const target = removalTargetForHost(host({ control_path: controlPath }));
      expect(target).toMatchObject({
        type: "managed_host",
        id: "host-1",
        registeredConnectorId: "connector-host-1",
        installMethod: "ssh",
        sshPath: "direct_executor",
        onboardingState: "registered",
      });
    },
  );

  it("keeps registered via-Bastion Host metadata", () => {
    const target = removalTargetForHost(
      host({
        control_path: "bastion_relay",
        bastion_scope_id: "scope-1",
        onboarding: {
          state: "registered",
          install_method: "ssh",
          ssh_path: "bastion_connector",
          updated_at: timestamp,
        },
      }),
    );
    expect(target).toMatchObject({
      registeredConnectorId: "connector-host-1",
      sshPath: "bastion_connector",
      bastionScopeId: "scope-1",
      onboardingState: "registered",
    });
  });

  it("uses the Bastion Scope active Connector as registration fact", () => {
    const scope = {
      id: "scope-1",
      enterprise_id: "enterprise-1",
      name: "scope-1",
      environment: "development",
      labels: {},
      status: "active",
      onboarding_mode: "direct_install_tunnel",
      onboarding: { state: "registered", updated_at: timestamp },
      connector_host_id: "host-1",
      active_connector_id: "connector-bastion-1",
      relay_https_port: 8445,
      relay_gateway_port: 9445,
      relay_port_generation: 1,
      relay_status: "ready",
      fencing_generation: 1,
      member_count: 0,
      resource_version: 4,
      created_at: timestamp,
      updated_at: timestamp,
    } satisfies BastionScope;
    const target = removalTargetForBastion(scope, host({ role: "bastion" }));
    expect(target).toMatchObject({
      type: "bastion_scope",
      registeredConnectorId: "connector-bastion-1",
      installMethod: "ssh",
      sshPath: "direct_executor",
      connectionStatus: "online",
      onboardingState: "registered",
    });
  });
});
