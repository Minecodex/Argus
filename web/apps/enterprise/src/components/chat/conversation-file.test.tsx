// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider, createMockApiClient } from "@argus/api-client";
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
    expect(screen.getByText("此格式暂不支持在线预览，请下载查看。")).toBeInTheDocument();
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
