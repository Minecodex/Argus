// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeAll, expect, it } from "vitest";
import { FileTextPreview } from "./file-preview";
import { filePreviewKind } from "./file-preview-format";
import { formatJsonPreview, parseDelimited } from "./file-preview-data";
import { readFilePreview } from "./file-preview-content";
import { LocaleProvider } from "./locale";

afterEach(cleanup);
beforeAll(async () => {
  await import("./markdown-content");
});

it.each([
  ["Human report title", "text/markdown", "markdown"],
  ["导出", "text/csv; charset=UTF-8", "csv"],
  ["payload", "application/problem+json", "json"],
  ["records", "application/x-ndjson", "jsonl"],
  ["file.tsv", undefined, "tsv"],
  ["file.toml", undefined, "code"],
  ["file.go", "text/plain", "code"],
  ["page.html", "text/html", "code"],
  ["drawing.svg", "image/svg+xml", "code"],
  ["picture", "image/png", "image"],
  ["document.pdf", "application/pdf", "unsupported"],
  ["data.csv", "application/octet-stream", "unsupported"],
  ["archive.md", "application/zip", "unsupported"],
])("classifies %s using trustworthy type metadata", (name, mime, kind) => {
  expect(filePreviewKind(name as string, mime as string | undefined)).toBe(
    kind,
  );
});

it("uses a real file path when an untyped display name has no extension", () => {
  expect(filePreviewKind("Report", undefined, "/workspace/report.md")).toBe(
    "markdown",
  );
});

it("renders a published Markdown report whose title is not its filename", async () => {
  render(
    <FileTextPreview
      name="B12 Redis INFO 报告"
      mediaType="text/markdown"
      content={
        "# Native report\n\n**Measured**\n\n|Metric|Value|\n|---|---|\n|commands|37|\n\n<script>window.bad=true</script>"
      }
    />,
  );
  expect(
    await screen.findByRole("heading", { name: "Native report" }),
  ).toBeVisible();
  expect(screen.getByRole("table")).toHaveTextContent("commands37");
  expect(document.querySelector("script")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "原文" }));
  expect(screen.queryByRole("heading", { name: "Native report" })).toBeNull();
  expect(screen.getByText(/# Native report/).tagName).toBe("PRE");
});

it("parses quoted delimiters, escaped quotes, embedded CRLF, BOM, blank rows and trailing fields", () => {
  const value = parseDelimited(
    '\uFEFFname,note,last\r\n\r\n"first, item","line1\r\nline2 ""quoted""",\r\n',
    ",",
  );
  expect(value.rows).toEqual([
    ["name", "note", "last"],
    ["first, item", 'line1\r\nline2 "quoted"', ""],
  ]);
});

it("renders untrusted CSV as text cells and allows headerless data", () => {
  render(
    <FileTextPreview
      name="data.csv"
      content={
        'name,value\n"<img src=x onerror=alert(1)>","=HYPERLINK(""https://example.test"")"\n'
      }
    />,
  );
  expect(screen.getByRole("columnheader", { name: "name" })).toBeVisible();
  expect(screen.getByRole("table")).toHaveTextContent(
    "<img src=x onerror=alert(1)>",
  );
  expect(document.querySelector("img,a")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "首行作为表头" }));
  expect(screen.getByRole("columnheader", { name: "列 1" })).toBeVisible();
  expect(screen.getAllByRole("row")).toHaveLength(3);
});

it("bounds rows/columns/cells and reports truncated previews without changing Raw", () => {
  const data =
    "key,value\n" +
    Array.from({ length: 205 }, (_, i) => `${i},value${i}`).join("\n");
  const { rerender } = render(
    <FileTextPreview name="data.csv" content={data} />,
  );
  expect(screen.getAllByRole("row")).toHaveLength(201);
  expect(screen.getByRole("status")).toHaveTextContent("200 行");
  fireEvent.click(screen.getByRole("button", { name: "原文" }));
  expect(screen.getByText(/204,value204/)).toHaveTextContent("204,value204");
  const wide = parseDelimited(
    Array.from({ length: 55 }, () => "x".repeat(5000)).join(","),
    ",",
  );
  expect(wide.columnsLimited && wide.cellsLimited).toBe(true);
  expect(wide.rows[0]).toHaveLength(50);
  rerender(<FileTextPreview name="invalid.csv" content={'a,b\n"unclosed,b'} />);
  fireEvent.click(screen.getByRole("button", { name: "预览" }));
  expect(screen.getByRole("status")).toHaveTextContent("无法按声明格式解析");
});

it("preserves exact JSON integer/exponent/string lexemes and formats JSONL", () => {
  const raw = String.raw`{"large":900719925474099312345,"a":1e400,"s":"comma, brace} escaped\""}`;
  const formatted = formatJsonPreview(raw);
  expect(formatted).toContain("900719925474099312345");
  expect(formatted).toContain("1e400");
  expect(formatted).toContain(String.raw`"s": "comma, brace} escaped\""`);
  render(
    <FileTextPreview
      name="records"
      mediaType="application/x-ndjson"
      content={'{"id":900719925474099312345}\n{"id":2}'}
    />,
  );
  expect(screen.getByText(/900719925474099312345/)).toHaveTextContent(
    '"id": 2',
  );
});

it("keeps HTML/SVG/code inert and provides localized controls", () => {
  render(
    <LocaleProvider locale="en-US" persist={false}>
      <FileTextPreview
        name="unsafe.svg"
        mediaType="image/svg+xml"
        content={
          '<svg><script>alert(1)</script><image href="https://example.test/private"/></svg>'
        }
      />
    </LocaleProvider>,
  );
  expect(
    screen.getByRole("button", { name: "Raw" }),
  ).toBeVisible();
  expect(document.querySelector("script,image,iframe")).toBeNull();
  expect(screen.getByRole("region", { name: "unsafe.svg" })).toHaveTextContent(
    "<svg>",
  );
});

it("decodes BOM-marked UTF-16 and rejects binary/invalid UTF-8 instead of corrupting it", async () => {
  const decoded = await readFilePreview(
    new Blob([new Uint8Array([255, 254, 65, 0, 44, 0, 66, 0])]),
    "csv",
  );
  expect(decoded).toEqual({ kind: "text", text: "A,B" });
  await expect(
    readFilePreview(new Blob([new Uint8Array([255, 0, 128])]), "text"),
  ).rejects.toThrow();
  await expect(
    readFilePreview(
      new Blob(['<svg onload="alert(1)"/>'], { type: "image/png" }),
      "image",
    ),
  ).rejects.toThrow();
  const png = await readFilePreview(
    new Blob([new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10, 0])]),
    "image",
  );
  expect(png.kind).toBe("image");
});
