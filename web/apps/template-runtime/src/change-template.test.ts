// @vitest-environment jsdom
import { afterEach, expect, it } from "vitest";
import { presentTool } from "./test-support";

afterEach(() => document.body.replaceChildren());

for (const locale of ["zh-CN", "en-US"]) {
  it("renders the actual resource update preview structure / " + locale, () => {
    presentTool(
      "host.update.preview",
      {
        title: "Update host",
        preview: {
          name: "old host",
          changes: [
            { path: "name", before: "old host", after: "new host <script>" },
          ],
        },
        diff: [{ kind: "change", text: "Update host old host" }],
      },
      locale,
    );
    expect(document.querySelector("table")?.textContent).toContain(
      "new host <script>",
    );
    expect(document.querySelector("script")).toBeNull();
    expect(document.querySelectorAll("table tbody tr")).toHaveLength(1);
  });

  it("preserves native kind/text operation facts / " + locale, () => {
    presentTool(
      "host.delete.preview",
      {
        preview: { name: "old host" },
        diff: [{ kind: "remove", text: "Delete host old host" }],
      },
      locale,
    );
    expect(document.body.textContent).toContain("Delete host old host");
    expect(document.querySelector("table")).toBeNull();
  });
}
