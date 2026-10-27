// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  act,
} from "@testing-library/react";
import { afterEach, afterAll, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  ApiProvider,
  createMockApiClient,
  type KubernetesInventoryObject,
} from "@argus/api-client";
import { LocaleProvider } from "@argus/ui";
import "../../i18n";
import { KubernetesMutationDialog } from "./mutation-dialog";

const caches: QueryClient[] = [];
afterEach(() => {
  cleanup();
  for (const cache of caches.splice(0)) cache.clear();
});
vi.stubGlobal(
  "ResizeObserver",
  class {
    observe() {}
    unobserve() {}
    disconnect() {}
  },
);
const previousAnimations = Element.prototype.getAnimations;
Element.prototype.getAnimations = () => [];
afterAll(() => {
  vi.unstubAllGlobals();
  if (previousAnimations) Element.prototype.getAnimations = previousAnimations;
  else Reflect.deleteProperty(Element.prototype, "getAnimations");
});

for (const backgroundRefresh of [false, true])
  it(`keeps the original edit version when the server changes: background=${backgroundRefresh}`, async () => {
    const api = createMockApiClient({ persist: false, delay: 0 });
    let resource: KubernetesInventoryObject = {
      definition: {
        key: "configmap",
        group: "",
        version: "v1",
        resource: "configmaps",
        kind: "ConfigMap",
        namespaced: true,
        category: "configuration",
        manage_permission: "kubernetes.configuration.manage",
      },
      uid: "stable-uid",
      namespace: "orders",
      namespace_uid: "namespace-uid",
      name: "settings",
      resource_version: "1",
      generation: 1,
      labels: {},
      owners: [],
      created_at: new Date().toISOString(),
      observed_at: new Date().toISOString(),
      redacted: false,
      data: {
        apiVersion: "v1",
        kind: "ConfigMap",
        metadata: {
          name: "settings",
          namespace: "orders",
          uid: "stable-uid",
          resourceVersion: "1",
        },
        data: { mode: "initial" },
      },
    };
    vi.spyOn(api.kubernetesWorkbench, "object").mockImplementation(async () =>
      structuredClone(resource),
    );
    const preview = vi.spyOn(api.kubernetesWorkbench, "preview");
    const cache = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    caches.push(cache);
    render(
      <LocaleProvider locale="zh-CN" persist={false}>
        <ApiProvider client={api}>
          <QueryClientProvider client={cache}>
            <KubernetesMutationDialog
              clusterId="cluster"
              definition={resource.definition}
              object={structuredClone(resource)}
              namespace={{ id: "namespace", name: "orders" }}
              onClose={() => {}}
              onSubmitted={() => {}}
            />
          </QueryClientProvider>
        </ApiProvider>
      </LocaleProvider>,
    );
    const editor = await screen.findByRole("textbox", {
      name: "对象清单（YAML / JSON）",
    });
    await waitFor(() =>
      expect((editor as HTMLTextAreaElement).value).toContain("mode: initial"),
    );
    expect((editor as HTMLTextAreaElement).value).not.toContain(
      "resourceVersion",
    );
    fireEvent.change(editor, {
      target: { value: "data:\n  mode: my-unsaved-edit\n" },
    });
    resource = {
      ...resource,
      resource_version: "2",
      data: {
        ...resource.data,
        metadata: {
          name: "settings",
          namespace: "orders",
          uid: "stable-uid",
          resourceVersion: "2",
        },
        data: { mode: "remote-edit" },
      },
    };
    if (backgroundRefresh)
      await act(async () => {
        await cache.invalidateQueries({
          queryKey: [
            "kubernetes-workbench",
            "cluster",
            "mutation-object",
            "stable-uid",
          ],
        });
      });
    else
      fireEvent.click(screen.getByRole("button", { name: "验证并预览差异" }));
    await screen.findAllByText(
      "资源已更新。当前编辑仍保留在旧版本，请重新加载后再预览。",
    );
    expect(editor).toHaveValue("data:\n  mode: my-unsaved-edit\n");
    expect(preview).not.toHaveBeenCalled();
    expect(
      screen.getByRole("button", { name: "验证并预览差异" }),
    ).toBeDisabled();
    fireEvent.click(
      screen.getByRole("button", { name: "重新加载并替换当前编辑" }),
    );
    await waitFor(() =>
      expect((editor as HTMLTextAreaElement).value).toContain("mode: remote-edit"),
    );
  });
