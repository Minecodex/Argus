// @vitest-environment jsdom
import { expect, it } from "vitest";
import { fixtures } from "./test-support";

it("queues many result roots within the host concurrent-read limit", async () => {
  const binding = fixtures.bindings.find(
    (row) => row.Tool === "telemetry.skywalking.trace",
  )!;
  const parsed = document.createElement("template");
  parsed.innerHTML =
    fixtures.assets[binding.Hash as keyof typeof fixtures.assets];
  let active = 0,
    peak = 0,
    calls = 0;
  const api = {
    onData: () => {},
    onDataChange: () => () => {},
    read: async () => {
      calls++;
      active++;
      peak = Math.max(peak, active);
      await new Promise((resolve) => setTimeout(resolve, 1));
      active--;
      return { data: { items: [], offset: 0 } };
    },
  };
  new Function(
    "ArgusTemplate",
    parsed.content.querySelector("script")!.textContent!,
  )(api);
  const view = (
    window as unknown as {
      ArgusView: {
        pages: (
          root: HTMLElement,
          data: unknown,
          render: () => void,
          parameters: unknown,
        ) => Promise<void>;
      };
    }
  ).ArgusView;
  await Promise.all(
    Array.from({ length: 12 }, (_, index) =>
      view.pages(document.createElement("div"), { items: [] }, () => {}, {
        collection: "result",
        index,
        field: "rows",
      }),
    ),
  );
  expect(calls).toBe(12);
  expect(peak).toBe(1);
});
