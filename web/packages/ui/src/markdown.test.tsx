// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeAll, expect, it, vi } from "vitest";
import { Markdown } from "./markdown";
import { FileCard, FileTextPreview } from "./file-card";
afterEach(cleanup);
beforeAll(async () => {
  await import("./markdown-content");
});
it("renders report Markdown including GFM tables and code without executing HTML or loading remote images", async () => {
  const { container } = render(
    <Markdown>
      {
        "## 主机报告\n\n**活跃告警**\n\n| 指标 | 数值 |\n|---|---|\n| CPU | 3.5% |\n\n```promql\nrate(cpu[5m])\n```\n\n<script>alert(1)</script>\n\n[unsafe](javascript:alert(1))\n\n![remote](https://example.com/image.png)"
      }
    </Markdown>,
  );
  expect(
    await screen.findByRole("heading", { name: "主机报告" }),
  ).toBeInTheDocument();
  expect(screen.getByRole("table")).toHaveTextContent("CPU3.5%");
  expect(container.querySelector("strong")).toHaveTextContent("活跃告警");
  expect(container.querySelector("code")).toHaveTextContent("rate(cpu[5m])");
  expect(container.querySelector("script,img")).toBeNull();
  expect(screen.getByText("unsafe")).not.toHaveAttribute(
    "href",
    expect.stringContaining("javascript:"),
  );
});
it("opens file previews from the filename and preview action while keeping downloading explicit", () => {
  const onPreview = vi.fn();
  render(
    <FileCard
      name="主机报告.md"
      size={128}
      downloadHref="/api/file"
      onPreview={onPreview}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "主机报告.md" }));
  fireEvent.click(
    screen.getByRole("button", { name: /^(预览|Preview) 主机报告.md$/ }),
  );
  expect(onPreview).toHaveBeenCalledTimes(2);
  expect(screen.getByRole("link")).toHaveAttribute("download", "主机报告.md");
});
it("renders Markdown files and formats JSON as read-only text", async () => {
  const { rerender } = render(
    <FileTextPreview name="报告.md" content="## 在线报告" />,
  );
  expect(
    await screen.findByRole("heading", { name: "在线报告" }),
  ).toBeInTheDocument();
  rerender(<FileTextPreview name="query.json" content={'{"value":3.5}'} />);
  expect(screen.getByText(/"value": 3.5/).tagName).toBe("PRE");
});
