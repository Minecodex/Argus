// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { LocaleProvider } from "./locale";
import { ObservationPanel } from "./observation-panel";

afterEach(cleanup);
it.each(["zh-CN", "en-US"])(
  "explains verified Namespace coverage in %s without claiming data loss",
  async (locale) => {
    localStorage.setItem("argus.locale", locale);
    render(
      <LocaleProvider>
        <ObservationPanel
          title="Namespace"
          signal="metrics"
          type="stat"
          targets={[
            {
              id: "main",
              status: "no_data",
              result_type: "vector",
              data: { result: [] },
              meta: {
                warnings: [
                  "NAMESPACE_VERIFIED_OBSERVATIONS_ONLY: qualification",
                ],
              },
            },
          ]}
        />
      </LocaleProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: /Namespace ·/ }));
    await screen.findByRole("dialog");
    expect(
      screen.getByText(
        locale === "zh-CN"
          ? /Namespace 指标仅包含已验证 UID/
          : /Namespace metrics include only observations/,
      ),
    ).toBeVisible();
    expect(
      screen.queryByText(/NAMESPACE_VERIFIED_OBSERVATIONS_ONLY/),
    ).not.toBeInTheDocument();
  },
);
