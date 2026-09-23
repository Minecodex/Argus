// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import {
  ApiError,
  ApiProvider,
  createMockApiClient,
  type Workspace,
} from "@argus/api-client";
import { LocaleProvider } from "@argus/ui";
import "../../i18n";
import { ConversationWorkspace } from "./conversation-workspace";

window.localStorage.setItem("argus.locale", "zh-CN");
afterEach(cleanup);
const value: Workspace = {
  id: "workspace",
  conversation_id: "conversation",
  status: "ready",
  capacity_bytes: 2147483648,
  version: 1,
  created_at: "2026-09-21T00:00:00Z",
};
const missing = () =>
  new ApiError(
    {
      code: "WORKSPACE_NOT_FOUND",
      message_key: "planv5.files.empty",
      request_id: "test",
      retryable: false,
    },
    404,
  );

async function setup(status: Workspace["status"] | null) {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  let current = status;
  vi.spyOn(api.workspace, "listFiles").mockResolvedValue([]);
  vi.spyOn(api.workspace, "get").mockImplementation(async () => {
    if (current === null) throw missing();
    return { ...value, status: current };
  });
  const remove = vi
    .spyOn(api.workspace, "remove")
    .mockImplementation(async () => {
      current = null;
      return { ...value, status: "deleted" };
    });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  render(
    <LocaleProvider>
      <ApiProvider client={api}>
        <QueryClientProvider client={client}>
          <ConversationWorkspace
            conversationId="conversation"
            selected={[]}
            disabled={false}
          />
        </QueryClientProvider>
      </ApiProvider>
    </LocaleProvider>,
  );
  fireEvent.click(screen.getByText("会话文件 · 0"));
  return { remove, api };
}

it("deletes an existing workspace with only generated files or deliveries", async () => {
  const { remove } = await setup("ready");
  const button = screen.getByRole("button", {
    name: "删除 Workspace",
  });
  await waitFor(() => expect(button).toBeEnabled());
  fireEvent.click(button);
  fireEvent.click(
    within(screen.getByRole("dialog")).getByRole("button", {
      name: "删除 Workspace",
    }),
  );
  await waitFor(() => expect(remove).toHaveBeenCalledWith("conversation"));
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  await waitFor(() => expect(button).toBeDisabled());
});

it.each(["failed", "provisioning"] as const)(
  "can explicitly clean an empty %s workspace",
  async (status) => {
    await setup(status);
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "删除 Workspace" }),
      ).toBeEnabled(),
    );
  },
);

it.each([null, "deleting", "deleted"] as const)(
  "cannot delete an absent or terminal workspace (%s)",
  async (status) => {
    const { api } = await setup(status);
    await waitFor(() => expect(api.workspace.get).toHaveBeenCalled());
    expect(
      screen.getByRole("button", { name: "删除 Workspace" }),
    ).toBeDisabled();
  },
);
