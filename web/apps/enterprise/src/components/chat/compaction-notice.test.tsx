// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import i18n from "i18next";
import "../../i18n";
import { CompactionNotice } from "./compaction-notice";

afterEach(cleanup);
for (const locale of ["zh-CN", "en-US"]) {
  it(`labels estimates and keeps missing counts unavailable in ${locale}`, async () => {
    await i18n.changeLanguage(locale);
    const view = render(
      <CompactionNotice
        value={{
          status: "completed",
          tokens_before: 21000,
          tokens_after: 1200,
          estimated: true,
        }}
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      locale === "zh-CN" ? "估算" : "estimated",
    );
    expect(screen.getByRole("status")).toHaveTextContent("21000 → 1200");
    view.rerender(<CompactionNotice value={{ status: "completed" }} />);
    expect(screen.getByRole("status")).toHaveTextContent(
      locale === "zh-CN" ? "统计不可用" : "statistics are unavailable",
    );
    expect(screen.getByRole("status")).not.toHaveTextContent("0 → 0");
    view.rerender(<CompactionNotice value={{ status: "running" }} />);
    expect(screen.getByRole("status")).not.toHaveTextContent("0");
    view.rerender(
      <CompactionNotice
        value={{
          status: "completed",
          tokens_before: 0,
          tokens_after: 0,
          estimated: true,
        }}
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent("0 → 0");
  });
}
