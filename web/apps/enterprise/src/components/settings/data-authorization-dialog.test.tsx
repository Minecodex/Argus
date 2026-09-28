// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider, createMockApiClient } from "@argus/api-client";
import { LocaleProvider } from "@argus/ui";
import "../../i18n";
import { DataAuthorizationDialog } from "./data-authorization-dialog";
afterEach(cleanup);
it("adds a dashboard grant without converting inherited resource grants into direct grants", async () => {
  localStorage.setItem("argus.locale", "zh-CN");
  const api = createMockApiClient({ persist: false, delay: 0 });
  const update = vi
    .spyOn(api.org, "updateDataAuthorization")
    .mockResolvedValue();
  vi.spyOn(api.org, "listDataAuthorization").mockImplementation(
    async (_subject, _id, type) => ({
      authorization_version: 1,
      affected_member_count: 1,
      page: { has_more: false, next_cursor: null },
      items:
        type === "host"
          ? [
              {
                resource_type: type,
                resource_id: "host",
                name: "Inherited host",
                direct: false,
                inherited: true,
                sources: ["Department"],
              },
            ]
          : type === "dashboard"
            ? [
                {
                  resource_type: type,
                  resource_id: "dashboard",
                  name: "Application dashboard",
                  direct: false,
                  inherited: false,
                  sources: [],
                },
              ]
            : [],
    }),
  );
  const cache = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <LocaleProvider>
      <ApiProvider client={api}>
        <QueryClientProvider client={cache}>
          <DataAuthorizationDialog
            open
            subjectType="user"
            subjectId="user"
            subjectLabel="Viewer"
            onOpenChange={() => {}}
          />
        </QueryClientProvider>
      </ApiProvider>
    </LocaleProvider>,
  );
  expect(
    await screen.findByRole("checkbox", { name: /Inherited host/ }),
  ).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "仪表盘" }));
  fireEvent.click(
    screen.getByRole("checkbox", { name: "Application dashboard" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "批量移动" }));
  fireEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(update).toHaveBeenCalledTimes(1));
  expect(update).toHaveBeenCalledWith(
    "user",
    "user",
    "dashboard",
    ["dashboard"],
    false,
    1,
  );
});
