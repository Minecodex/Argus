import type { TooltipComponentOption } from "echarts/components";
import { observationTooltipContent } from "./observation-tooltip-content";

export function fitObservationTooltip(
  point: number[],
  size: number[],
  origin: number[],
  viewport: number[],
  gap: number,
) {
  return [0, 1].map(
    (axis) =>
      Math.max(
        gap,
        Math.min(
          origin[axis]! + point[axis]!,
          Math.max(gap, viewport[axis]! - size[axis]! - gap),
        ),
      ) - origin[axis]!,
  );
}

/** Canvas tooltips are clipped by chart bounds; use one document-level layer. */
export function floatingObservationTooltip(
  option: TooltipComponentOption = {},
  element?: HTMLElement,
  structuredRows = true,
): TooltipComponentOption {
  let host: HTMLElement | undefined = element;
  return {
    ...option,
    renderMode: "html",
    appendTo: (container) => {
      host = container;
      return container.ownerDocument.body;
    },
    confine: false,
    className: "argus-chart-tooltip",
    enterable: true,
    formatter:
      option.formatter ??
      (structuredRows
        ? (params) =>
            observationTooltipContent(
              params,
              host?.ownerDocument ?? document,
              option.valueFormatter,
            )
        : undefined),
    position: (point, params, element, rect, sizes) => {
      if (!host) return point;
      const doc = host.ownerDocument,
        view = doc.defaultView!;
      const origin = host.getBoundingClientRect();
      const gap =
        Number.parseFloat(
          view.getComputedStyle(host).getPropertyValue("--space-3"),
        ) || 0;
      const viewport = [
        doc.documentElement.clientWidth,
        doc.documentElement.clientHeight,
      ];
      const content =
        element && "getBoundingClientRect" in element
          ? element.getBoundingClientRect()
          : undefined;
      const size = content
        ? [content.width, content.height]
        : sizes.contentSize;
      const specified =
        typeof option.position === "function"
          ? option.position(point, params, element, rect, sizes)
          : option.position;
      let desired = [point[0] + gap, point[1] + gap];
      if (Array.isArray(specified))
        desired = specified.map((v, i) =>
          typeof v === "string" && v.endsWith("%")
            ? (Number.parseFloat(v) * sizes.viewSize[i]!) / 100
            : Number(v),
        );
      else if (!specified) {
        if (origin.left + desired[0]! + size[0]! + gap > viewport[0]!)
          desired[0] = point[0] - size[0]! - gap;
        if (origin.top + desired[1]! + size[1]! + gap > viewport[1]!)
          desired[1] = point[1] - size[1]! - gap;
      } else return specified;
      return fitObservationTooltip(
        desired,
        size,
        [origin.left, origin.top],
        viewport,
        gap,
      );
    },
  };
}
