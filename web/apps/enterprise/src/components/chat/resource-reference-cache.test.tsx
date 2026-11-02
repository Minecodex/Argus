// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider, createMockApiClient } from "@argus/api-client";
import { afterEach, expect, it, vi } from "vitest";
import { ResourceReferenceDetails } from "./resource-reference-details";
import { useResourceMentions } from "./use-resource-mentions";

vi.mock("../../lib/permissions", () => ({
  useMyPermissions: () => new Set(["application.read"]),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
afterEach(cleanup);

function Mentions() {
  const result = useResourceMentions("orders", true);
  return <>{result.items.map((item) => <span key={item.id}>{item.label}</span>)}</>;
}

it("hides cached details until the fresh authorization check finishes and hides inaccessible data", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  let finish!: (value: { items: [] }) => void;
  const read = vi.spyOn(api.telemetry, "resources").mockReturnValue(
    new Promise((resolve) => { finish = resolve; }),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(["chat-resource-reference", "application", "app"], {
    type: "application", id: "app", name: "cached private details",
  });
  render(<ApiProvider client={api}><QueryClientProvider client={client}>
    <ResourceReferenceDetails type="application" id="app" />
  </QueryClientProvider></ApiProvider>);
  await waitFor(() => expect(read).toHaveBeenCalledTimes(1));
  expect(screen.queryByText("cached private details")).toBeNull();
  expect(screen.getByRole("status").textContent).toContain("chat.composer.resourceLoading");
  finish({ items: [] });
  await screen.findByRole("alert");
  expect(screen.queryByText("cached private details")).toBeNull();
  client.clear();
});

it("hides stale mention names while revalidating and keeps a revoked directory empty", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  let finish!: (value: { items: [] }) => void;
  const read = vi.spyOn(api.telemetry, "resources").mockReturnValue(
    new Promise((resolve) => { finish = resolve; }),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(["chat-resource-mentions", "orders", "application"], {
    pages: [{ items: [{ kind: "application", id: "app", label: "cached private mention" }], cursors: { application: null } }],
    pageParams: [{}],
  });
  render(<ApiProvider client={api}><QueryClientProvider client={client}><Mentions /></QueryClientProvider></ApiProvider>);
  await waitFor(() => expect(read).toHaveBeenCalledTimes(1));
  expect(screen.queryByText("cached private mention")).toBeNull();
  finish({ items: [] });
  await waitFor(() => expect(client.isFetching()).toBe(0));
  expect(screen.queryByText("cached private mention")).toBeNull();
  client.clear();
});
