import type {
  ConnectionTest,
  HostConnectionTestCreate,
} from "../generated/contracts";
import { ApiError } from "../transport/errors";
import type { MockContext } from "./context";
import { nextId } from "./store";

type Evidence = { input: HostConnectionTestCreate; result: ConnectionTest };
type ProbeInput = Partial<HostConnectionTestCreate> & {
  connection_test_id?: string;
};

/** One registry lets Host and Bastion previews verify the same frozen SSH evidence. */
export function createHostConnectionTests(ctx: MockContext) {
  const tests = new Map<string, Evidence>();
  return {
    async create(input: HostConnectionTestCreate): Promise<ConnectionTest> {
      const frozen = { ...input };
      await ctx.pause();
      const now = ctx.nowIso();
      const id = nextId(ctx.db, "ct");
      const result: ConnectionTest = {
        id,
        enterprise_id: ctx.enterpriseId(),
        target_type: "host",
        path: frozen.ssh_path === "bastion_connector" ? "connector" : "direct",
        platform: frozen.platform,
        architecture: "amd64",
        status: "succeeded",
        checks: [
          { name: "dns_resolve", status: "passed", detail: frozen.address },
          { name: "authentication", status: "passed" },
          ...(frozen.onboarding_control_path
            ? [
                {
                  name: "onboarding_callback",
                  status: "passed" as const,
                  detail: frozen.onboarding_control_path,
                },
              ]
            : []),
        ],
        latency_ms: 120,
        expires_at: new Date(Date.now() + 600_000).toISOString(),
        created_at: now,
        updated_at: now,
      };
      tests.set(id, { input: frozen, result });
      return structuredClone(result);
    },
    async get(id: string): Promise<ConnectionTest> {
      await ctx.pause();
      const evidence = tests.get(id);
      if (!evidence || evidence.result.enterprise_id !== ctx.enterpriseId())
        throw new Error("connection test not found");
      return structuredClone(evidence.result);
    },
    requireOnboarding(input: ProbeInput) {
      const evidence = tests.get(input.connection_test_id ?? "");
      const fields = [
        "address",
        "port",
        "username",
        "credential_id",
        "platform",
        "ssh_path",
        "bastion_scope_id",
        "onboarding_control_path",
      ] as const;
      if (
        !evidence ||
        !input.onboarding_control_path ||
        evidence.result.enterprise_id !== ctx.enterpriseId() ||
        evidence.result.status !== "succeeded" ||
        Date.parse(evidence.result.expires_at) <= Date.now() ||
        fields.some((key) => evidence.input[key] !== input[key]) ||
        !evidence.result.checks.some(
          (check) =>
            check.name === "onboarding_callback" &&
            check.status === "passed" &&
            check.detail === input.onboarding_control_path,
        )
      ) {
        throw new ApiError(
          {
            code: "CONNECTION_TEST_REQUIRED",
            message_key: "errors.resource.connection_test_required",
            request_id: "mock-callback-preflight",
            retryable: false,
          },
          409,
        );
      }
    },
  };
}

export type HostConnectionTests = ReturnType<typeof createHostConnectionTests>;
