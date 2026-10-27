// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, afterAll, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider, createMockApiClient } from "@argus/api-client";
import { LocaleProvider } from "@argus/ui";
import "../../i18n";
import { DataAuthorizationDialog } from "./data-authorization-dialog";
import { within } from "@testing-library/react";
afterEach(cleanup);
vi.stubGlobal(
  "ResizeObserver",
  class {
    observe() {}
    unobserve() {}
    disconnect() {}
  },
);
const originalAnimations = Element.prototype.getAnimations;
Element.prototype.getAnimations = () => [];
afterAll(() => {
  vi.unstubAllGlobals();
  if (originalAnimations) Element.prototype.getAnimations = originalAnimations;
  else Reflect.deleteProperty(Element.prototype, "getAnimations");
});

for (const changedVersion of [false, true])
  it(`preserves cross-page edits and detects changed authorization version: ${changedVersion}`, async () => {
    const api = createMockApiClient({ persist: false, delay: 0 });
    let version = 1;
    const update = vi
      .spyOn(api.org, "updateDataAuthorization")
      .mockImplementation(async () => {
        version++;
      });
    const read = vi
      .spyOn(api.org, "listDataAuthorization")
      .mockImplementation(async (_subject, _id, type, cursor) => ({
        authorization_version: cursor && changedVersion ? 2 : version,
        affected_member_count: 1,
        page: { has_more: !cursor, next_cursor: cursor ? null : "next" },
        items: [
          {
            resource_type: type,
            resource_id: cursor ? "second" : "first",
            name: cursor ? "Second page resource" : "First page resource",
            direct: false,
            inherited: false,
            sources: [],
          },
        ],
      }));
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
              subjectId="subject"
              subjectLabel="Operator"
              onOpenChange={() => {}}
            />
          </QueryClientProvider>
        </ApiProvider>
      </LocaleProvider>,
    );
    await screen.findByText("First page resource");
    expect(read).toHaveBeenCalledTimes(1);
    fireEvent.click(
      within(
        screen.getByRole("row", { name: /First page resource/ }),
      ).getByRole("button", { name: "授予" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "下一页" }));
    await screen.findByText("Second page resource");
    if (changedVersion) {
      await screen.findByText("授权已被其他操作修改。请重新加载后核对再保存。");
      fireEvent.click(screen.getByRole("button", { name: "保存" }));
      expect(update).not.toHaveBeenCalled();
    } else {
      fireEvent.click(
        within(
          screen.getByRole("row", { name: /Second page resource/ }),
        ).getByRole("button", { name: "授予" }),
      );
      fireEvent.click(screen.getByRole("button", { name: "保存" }));
      await waitFor(() => expect(update).toHaveBeenCalledTimes(1));
      expect(update).toHaveBeenCalledWith(
        "user",
        "subject",
        "host",
        ["first", "second"],
        false,
        1,
      );
    }
  });
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
  await screen.findByText("Inherited host");
  expect(screen.getByRole("button", { name: "授予" })).toBeDisabled();
  fireEvent.click(screen.getByRole("tab", { name: "仪表盘" }));
  await screen.findByText("Application dashboard");
  fireEvent.click(screen.getByRole("button", { name: "授予" }));
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
