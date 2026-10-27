import { describe, expect, it } from "vitest";
import type { MonitoringSchemas } from "@argus/api-client";
import { sdkConnectionExample } from "./sdk-connection-guide";

const connection: MonitoringSchemas["MonitoringIngestConnection"] = {
  name: "SDK orders",
  endpoint_id: "10000000-0000-4000-8000-000000000001",
  application_id: "10000000-0000-4000-8000-000000000002",
  collector_id: "10000000-0000-4000-8000-000000000003",
  environment: "production",
  protocol: "otlp",
  signals: ["traces"],
  desired_enabled: true,
  configuration_status: "effective",
  collector_status: "converged",
  application_status: "active",
  observation_state: "waiting",
  last_error_code: "",
  authentication_header: "authorization",
  authentication_scheme: "Bearer",
  service_name_required: true,
  network_prerequisite:
    "resolve_collector_dns_or_use_local_address_with_tls_server_name",
  listeners: [
    {
      transport: "grpc",
      port: 24317,
      path: "",
      endpoint: "grpcs://collector.example.test:24317",
      local_endpoint: "grpcs://127.0.0.1:24317",
    },
    {
      transport: "http",
      port: 24318,
      path: "",
      endpoint: "https://collector.example.test:24318",
      local_endpoint: "https://127.0.0.1:24318",
    },
  ],
  tls: {
    required: true,
    minimum_version: "1.3",
    server_name: "collector.example.test",
    trust_bundle_epoch: 3,
    ca_bundle_pem: "PUBLIC_TEST_CERTIFICATE",
    ca_bundle_sha256: "0".repeat(64),
    ca_fingerprints: ["0".repeat(64)],
  },
};

describe("SDK connection examples", () => {
  it("quotes an arbitrary service name as a shell literal", () => {
    const value = sdkConnectionExample(
      connection,
      "http",
      "orders'$(printf unexpected)",
    );
    expect(value).toContain(
      "OTEL_SERVICE_NAME='orders'\\''$(printf unexpected)'",
    );
    expect(value).toContain("authorization=Bearer%20${ARGUS_APPLICATION_INGEST_KEY}");
    expect(value).not.toContain("argus_app.");
    expect(value).not.toContain("deployment.environment");
  });

  it("uses the canonical HTTPS address for standard OTLP gRPC variables", () => {
    const value = sdkConnectionExample(connection, "grpc", "orders");
    expect(value).toContain("OTEL_EXPORTER_OTLP_PROTOCOL='grpc'");
    expect(value).toContain(
      "OTEL_EXPORTER_OTLP_ENDPOINT='https://collector.example.test:24317'",
    );
    expect(value).toContain("OTEL_EXPORTER_OTLP_CERTIFICATE='argus-sdk-ca.pem'");
    expect(sdkConnectionExample(connection, "unknown", "orders")).toBe("");
  });
});
