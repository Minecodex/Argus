// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { useState } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { StructuredFieldsEditor } from "./structured-fields-editor";
afterEach(cleanup);
it.each([false, true])(
  "preserves existing multiline strings and edited files: explicit=%s",
  (multiline) => {
    const original = "server:\n  port: 8080\n",
      updated = "server:\n  port: 9090\n\n";
    let saved: Record<string, unknown> = {};
    function Form() {
      const [value, setValue] = useState({
        data: { "app.yaml": original, "other.txt": "untouched\n" },
        unknown: "keep",
      });
      saved = value;
      return (
        <StructuredFieldsEditor
          fields={[{ key: "data", label: "Files", kind: "map", multiline }]}
          value={value}
          onChange={(next) => setValue(next as typeof value)}
        />
      );
    }
    render(<Form />);
    const field = screen.getByRole("textbox", { name: "值 1" });
    expect(field.tagName).toBe("TEXTAREA");
    expect(field).toHaveValue(original);
    fireEvent.change(screen.getByRole("textbox", { name: "键 2" }), {
      target: { value: "renamed.txt" },
    });
    expect(saved).toEqual({
      data: { "app.yaml": original, "renamed.txt": "untouched\n" },
      unknown: "keep",
    });
    fireEvent.change(field, { target: { value: updated } });
    expect((saved.data as Record<string, string>)["app.yaml"]).toBe(updated);
  },
);
