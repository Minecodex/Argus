import { expect, it, vi } from "vitest";
import { createConfiguredApiClient } from "../../factory";

it("platform profiles configure the offline Workspace task and retain server versions", async () => {
  const writes: Record<string, unknown>[] = [];
  const image = {
    id: "image",
    backend_id: "backend",
    name: "Offline",
    image_ref: "registry/image",
    digest: "sha256:test",
    status: "enabled",
    created_at: "2026-09-13T00:00:00Z",
  };
  const fetch = vi.fn(
    async (input: string | URL | Request, init?: RequestInit) => {
      const path = new URL(String(input)).pathname;
      let response: unknown;
      if (path.endsWith("/images")) response = [image];
      else {
        const body = JSON.parse(String(init?.body)) as Record<string, unknown>;
        writes.push(body);
        expect(new Headers(init?.headers).has("X-CSRF-Token")).toBe(true);
        if (init?.method === "POST")
          expect(new Headers(init.headers).has("Idempotency-Key")).toBe(true);
        response = {
          ...body,
          id: "profile",
          version: writes.length,
          revision: writes.length,
          created_at: image.created_at,
        };
      }
      return new Response(JSON.stringify(response), {
        headers: { "Content-Type": "application/json" },
      });
    },
  );
  const api = await createConfiguredApiClient({
    portal: "platform",
    mode: "real",
    base_url: "https://api.example.test",
    csrf_token: () => "test-csrf",
    fetch,
  });
  const profile = await api.platform.profiles.create({
    name: "Offline analysis",
    imageId: "image",
    resources: { cpu: 1, memoryMb: 1024 },
    timeoutSeconds: 300,
  });
  expect(writes[0]).toMatchObject({
    task_kinds: ["agent_workspace"],
    network_mode: "none",
    backend_id: "backend",
    cpu_millis: 1000,
    memory_mib: 1024,
    timeout_seconds: 300,
  });
  expect(profile).toMatchObject({
    taskKinds: ["agent_workspace"],
    networkMode: "none",
    timeoutSeconds: 300,
  });
  await api.platform.profiles.update(profile.id, { enabled: false });
  expect(writes[1]).toMatchObject({
    task_kinds: ["agent_workspace"],
    network_mode: "none",
    expected_version: 1,
    status: "disabled",
  });
});

it("quota creation starts at version zero and preserves exact monthly seconds", async () => {
  let current: Record<string, unknown> | undefined;
  const writes: Record<string, unknown>[] = [];
  const fetch = vi.fn(
    async (_input: string | URL | Request, init?: RequestInit) => {
      if (init?.method === "PUT") {
        const body = JSON.parse(String(init.body)) as Record<string, unknown>;
        writes.push(body);
        current = {
          ...body,
          enterprise_id: "enterprise",
          version: writes.length,
        };
      }
      return new Response(
        JSON.stringify(
          current ?? {
            code: "SANDBOX_PROFILE_UNAVAILABLE",
            message_key: "errors.sandbox.profile_unavailable",
            request_id: "test",
            retryable: false,
          },
        ),
        {
          status: current ? 200 : 404,
          headers: { "Content-Type": "application/json" },
        },
      );
    },
  );
  const api = await createConfiguredApiClient({
    portal: "platform",
    mode: "real",
    base_url: "https://api.example.test",
    csrf_token: () => "test-csrf",
    fetch,
  });
  const initial = await api.platform.quotas.get("enterprise");
  expect(initial).toMatchObject({
    version: 0,
    maxConcurrentSessions: 0,
    monthlySessionSeconds: 0,
  });
  const created = await api.platform.quotas.update("enterprise", {
    expectedVersion: initial.version,
    maxConcurrentSessions: 2,
    monthlySessionSeconds: 125,
  });
  await api.platform.quotas.update("enterprise", {
    expectedVersion: created.version,
    maxConcurrentSessions: 3,
    monthlySessionSeconds: 125,
  });
  expect(writes).toEqual([
    {
      expected_version: 0,
      max_concurrent_sessions: 2,
      monthly_session_seconds: 125,
    },
    {
      expected_version: 1,
      max_concurrent_sessions: 3,
      monthly_session_seconds: 125,
    },
  ]);
  expect(fetch).toHaveBeenCalledTimes(3);
});
