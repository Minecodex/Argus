// @vitest-environment jsdom
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { ApiProvider, createMockApiClient } from "@argus/api-client";
import { useEnterpriseAuthStore } from "@argus/auth";
import { useMyRoles, usePermission } from "./permissions";

afterEach(() => {
  cleanup();
  useEnterpriseAuthStore.setState({ status: "anonymous", session: null });
});

it("uses self permissions without requiring enterprise role catalog access and rechecks revoked grants", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  const login = await api.auth.login({ username: "root", password: "123456" });
  const session = {
    ...login,
    permissions: ["telemetry.dashboard.read", "telemetry.dashboard.manage"],
  };
  const me = vi.spyOn(api.auth, "me").mockResolvedValue(session);
  const roles = vi
    .spyOn(api.org, "listRoles")
    .mockRejectedValue(new Error("role.read denied"));
  const bindings = vi
    .spyOn(api.org, "listRoleBindings")
    .mockRejectedValue(new Error("role.read denied"));
  useEnterpriseAuthStore.setState({ status: "authenticated", session });
  const query = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const { result } = renderHook(
    () => ({
      allowed: usePermission("telemetry.dashboard.manage"),
      roles: useMyRoles(),
    }),
    {
      wrapper: ({ children }: { children: ReactNode }) => (
        <ApiProvider client={api}>
          <QueryClientProvider client={query}>{children}</QueryClientProvider>
        </ApiProvider>
      ),
    },
  );
  await waitFor(() => expect(me).toHaveBeenCalled());
  expect(result.current.allowed).toBe(true);
  expect(result.current.roles).toEqual([]);
  expect(roles).not.toHaveBeenCalled();
  expect(bindings).not.toHaveBeenCalled();
  me.mockResolvedValue({ ...session, permissions: [] });
  await act(async () => {
    await query.invalidateQueries({ queryKey: ["org"] });
  });
  await waitFor(() => expect(result.current.allowed).toBe(false));

  // A different account must not inherit the first account's cached permissions.
  const other = {
    ...session,
    user: { ...session.user, id: "different-user" },
    session: {
      ...session.session,
      id: "different-session",
      user_id: "different-user",
    },
    permissions: [],
  };
  me.mockResolvedValue(other);
  act(() => useEnterpriseAuthStore.setState({ session: other }));
  expect(result.current.allowed).toBe(false);
  expect(roles).not.toHaveBeenCalled();
});
