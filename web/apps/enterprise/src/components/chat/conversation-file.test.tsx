// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiError, ApiProvider, createMockApiClient } from "@argus/api-client";
import { LocaleProvider } from "@argus/ui";
import { afterEach, beforeAll, expect, it, vi } from "vitest";
import "../../i18n";
import { ConversationFile } from "./conversation-file";
// The production editor loads Markdown lazily to keep the chat entry small.
// Resolve that module before testing the file read and modal behavior; retain
// the existing heading deadline for those user-visible operations.
beforeAll(async () => {
  await import("../../../../../packages/ui/src/markdown-content");
});
afterEach(cleanup);
it("rechecks authorization when reopening and removes formerly readable preview content after revocation", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  vi.spyOn(api.workspace, "downloadUrl").mockReturnValue("/api/v1/restricted");
  const read = vi
    .spyOn(api.workspace, "readContent")
    .mockResolvedValueOnce(
      new Blob(["# Confidential report"], { type: "text/markdown" }),
    )
    .mockRejectedValueOnce(
      new ApiError(
        {
          code: "WORKSPACE_SOURCE_DENIED",
          message_key: "planv5.files.unavailable",
          request_id: "revoked-read",
          retryable: false,
        },
        403,
      ),
    );
  render(
    <LocaleProvider>
      <ApiProvider client={api}>
        <QueryClientProvider client={new QueryClient()}>
          <ConversationFile
            conversationId="conversation"
            file={{
              id: "restricted",
              name: "report.md",
              byte_size: 21,
              media_type: "text/markdown",
            }}
          />
        </QueryClientProvider>
      </ApiProvider>
    </LocaleProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "预览 report.md" }));
  expect(
    await screen.findByRole("heading", { name: "Confidential report" }),
  ).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "关闭" }));
  fireEvent.click(screen.getByRole("button", { name: "预览 report.md" }));
  expect(await screen.findByRole("alert")).toBeVisible();
  expect(
    screen.queryByRole("heading", { name: "Confidential report" }),
  ).toBeNull();
  expect(read).toHaveBeenCalledTimes(2);
});
it.each(["archive.bin", "misleading.txt"])(
  "keeps binary %s downloadable without fetching it as text",
  async (name) => {
    const api = createMockApiClient({ persist: false, delay: 0 });
    vi.spyOn(api.workspace, "downloadUrl").mockReturnValue("/api/v1/binary");
    const read = vi.spyOn(api.workspace, "readContent");
    render(
      <LocaleProvider>
        <ApiProvider client={api}>
          <QueryClientProvider client={new QueryClient()}>
            <ConversationFile
              conversationId="conversation"
              file={{
                id: "binary",
                name,
                byte_size: 4,
                media_type: "application/octet-stream",
              }}
            />
          </QueryClientProvider>
        </ApiProvider>
      </LocaleProvider>,
    );
    expect(screen.getByText(/application\/octet-stream/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name }));
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    expect(
      screen.getByText("此格式暂不支持在线预览，请下载查看。"),
    ).toBeInTheDocument();
    expect(read).not.toHaveBeenCalled();
    expect(
      screen
        .getAllByRole("link", { name: /下载|Download/ })
        .every((link) => link.getAttribute("href") === "/api/v1/binary"),
    ).toBe(true);
  },
);
it("previews authorized report content in a large modal and keeps the file link as an explicit download", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  vi.spyOn(api.workspace, "downloadUrl").mockReturnValue("/api/v1/file");
  const read = vi.spyOn(api.workspace, "readContent").mockResolvedValue(
    new Blob(["## 主机报告\n\n| CPU | 内存 |\n|---|---|\n| 3.5% | 45% |"], {
      type: "text/markdown",
    }),
  );
  render(
    <LocaleProvider>
      <ApiProvider client={api}>
        <QueryClientProvider client={new QueryClient()}>
          <ConversationFile
            conversationId="conversation"
            file={{
              id: "report",
              name: "主机报告.md",
              byte_size: 70,
              media_type: "text/markdown",
            }}
          />
        </QueryClientProvider>
      </ApiProvider>
    </LocaleProvider>,
  );
  expect(read).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "主机报告.md" }));
  expect(
    await screen.findByRole("heading", { name: "主机报告" }),
  ).toBeInTheDocument();
  expect(screen.getByRole("dialog")).toHaveClass("argus-dialog--cover");
  expect(screen.getByRole("table")).toHaveTextContent("3.5%");
  expect(read).toHaveBeenCalledWith(
    "conversation",
    "report",
    "file",
    expect.any(AbortSignal),
  );
  expect(
    screen
      .getAllByRole("link", { name: /下载|Download/ })
      .every((link) => link.hasAttribute("download")),
  ).toBe(true);
});

it("renders a delivery's Markdown MIME despite its custom display title", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  vi.spyOn(api.workspace, "readContent").mockResolvedValue(
    new Blob(["# Delivery report\n\n**Verified**"], { type: "text/markdown" }),
  );
  render(
    <LocaleProvider>
      <ApiProvider client={api}>
        <QueryClientProvider client={new QueryClient()}>
          <ConversationFile
            conversationId="conversation"
            kind="delivery"
            file={{
              id: "delivery",
              name: "Custom report title",
              media_type: "text/markdown",
              byte_size: 40,
            }}
          />
        </QueryClientProvider>
      </ApiProvider>
    </LocaleProvider>,
  );
  fireEvent.click(
    screen.getByRole("button", { name: "预览 Custom report title" }),
  );
  expect(
    await screen.findByRole("heading", { name: "Delivery report" }),
  ).toBeVisible();
});

it("renders CSV attachment rows and retains the explicit original download", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  vi.spyOn(api.workspace, "downloadUrl").mockReturnValue("/api/v1/csv");
  vi.spyOn(api.workspace, "readContent").mockResolvedValue(
    new Blob(['name,value\n"name, quoted",001\n'], { type: "text/csv" }),
  );
  render(
    <LocaleProvider>
      <ApiProvider client={api}>
        <QueryClientProvider client={new QueryClient()}>
          <ConversationFile
            conversationId="conversation"
            file={{
              id: "csv",
              name: "Data",
              media_type: "text/csv",
              byte_size: 38,
            }}
          />
        </QueryClientProvider>
      </ApiProvider>
    </LocaleProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "预览 Data" }));
  expect(await screen.findByRole("table")).toHaveTextContent("name, quoted001");
  expect(
    screen
      .getAllByRole("link", { name: /下载/ })
      .every((link) => link.getAttribute("href") === "/api/v1/csv"),
  ).toBe(true);
});
