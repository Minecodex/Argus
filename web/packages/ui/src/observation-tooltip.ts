import type { TooltipComponentOption } from "echarts/components";

/** Canvas tooltips are clipped by chart bounds; use one document-level layer. */
export function floatingObservationTooltip(
  option: TooltipComponentOption = {},
): TooltipComponentOption {
  return {
    ...option,
    renderMode: "html",
    appendTo: (container) => container.ownerDocument.body,
    confine: false,
    className: "argus-chart-tooltip",
  };
}
