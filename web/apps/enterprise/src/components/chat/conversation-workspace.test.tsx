// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import {
  ApiError,
  ApiProvider,
  createMockApiClient,
  type WorkspaceFile,
} from "@argus/api-client";
import { LocaleProvider } from "@argus/ui";
import "../../i18n";
import { ConversationWorkspace } from "./conversation-workspace";

window.localStorage.setItem("argus.locale", "zh-CN");
afterEach(cleanup);

async function setup(files: WorkspaceFile[] = [], fileError?: ApiError) {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const listFiles = vi.spyOn(api.workspace, "listFiles");
  if (fileError) listFiles.mockRejectedValue(fileError);
  else listFiles.mockResolvedValue(files);
  const remove = vi.spyOn(api.workspace, "remove");
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
            expanded
          />
        </QueryClientProvider>
      </ApiProvider>
    </LocaleProvider>,
  );
  return { remove };
}

it("keeps existing files browsable without offering workspace deletion", async () => {
  const { remove } = await setup([
    {
      id: "report",
      name: "report.txt",
      path: "/workspace/report.txt",
      byte_size: 12,
      media_type: "text/plain",
      content_hash: "a".repeat(64),
      created_at: "2026-10-18T00:00:00Z",
    },
  ]);
  expect(
    await screen.findByRole("button", { name: "预览 report.txt" }),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: /删除 Workspace|Delete Workspace/ }),
  ).not.toBeInTheDocument();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(
    screen.getByText("文件随会话保留，永久删除会话时一并清理。"),
  ).toBeInTheDocument();
  expect(remove).not.toHaveBeenCalled();
});

it("shows an empty file list without a deletion action", async () => {
  const { remove } = await setup();
  expect(await screen.findByText("没有文件")).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: /删除 Workspace|Delete Workspace/ }),
  ).not.toBeInTheDocument();
  expect(remove).not.toHaveBeenCalled();
});

it("shows an empty list when the workspace has not been created", async () => {
  await setup(
    [],
    new ApiError(
      {
        code: "WORKSPACE_NOT_FOUND",
        message_key: "planv5.files.empty",
        request_id: "test",
        retryable: false,
      },
      404,
    ),
  );
  expect(await screen.findByText("没有文件")).toBeInTheDocument();
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});

it("reports file service failures instead of claiming the workspace is empty", async () => {
  await setup(
    [],
    new ApiError(
      {
        code: "WORKSPACE_STORAGE_UNAVAILABLE",
        message_key: "planv5.files.unavailable",
        request_id: "test",
        retryable: true,
      },
      503,
    ),
  );
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "暂时无法读取会话文件",
  );
  expect(screen.queryByText("没有文件")).not.toBeInTheDocument();
});
