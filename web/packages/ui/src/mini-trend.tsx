import { useState, type ReactNode } from "react";
import { Tooltip } from "@heroui/react/tooltip";

export type MiniTrendPoint = { time: string; value: number | null };

/** A small, gap-preserving trend with mouse and keyboard point inspection. */
export function MiniTrend<T extends MiniTrendPoint>({
  points,
  label,
  latest,
  reference,
  renderDetail,
}: {
  points: T[];
  label: string;
  latest: ReactNode;
  reference?: number;
  renderDetail: (point: T) => ReactNode;
}) {
  const [selected, setSelected] = useState<number>();
  const [open, setOpen] = useState(false);
  const finite = (value: number | null): value is number =>
    value !== null && Number.isFinite(value);
  const values = points.map((point) => point.value).filter(finite);
  const min = Math.min(0, ...values);
  const upper = Math.max(reference ?? 0, ...values);
  const max = Number.isFinite(upper) && upper > min ? upper : min + 1;
  const y = (value: number) => 28 - ((value - min) / (max - min)) * 24;
  const x = (index: number) =>
    points.length < 2 ? 60 : 2 + (index / (points.length - 1)) * 116;
  let path = "",
    segment = false;
  points.forEach((point, index) => {
    if (!finite(point.value)) {
      segment = false;
      return;
    }
    path += `${segment ? " L" : " M"}${x(index)},${y(point.value)}`;
    segment = true;
  });
  let last = points.length - 1;
  while (last > 0 && !finite(points[last]!.value)) last--;
  const active =
    points[Math.max(0, Math.min(selected ?? last, points.length - 1))];
  return (
    <Tooltip delay={0} closeDelay={100} isOpen={open} onOpenChange={setOpen}>
      <Tooltip.Trigger<"button">
        render={({ onKeyDown, ...props }) => (
          <button
            {...props}
            type="button"
            onKeyDown={(event) => {
              const current = selected ?? Math.max(0, last);
              if (
                !["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)
              ) {
                onKeyDown?.(event);
                return;
              }
              event.preventDefault();
              event.stopPropagation();
              setOpen(true);
              setSelected(
                event.key === "Home"
                  ? 0
                  : event.key === "End"
                    ? points.length - 1
                    : Math.max(
                        0,
                        Math.min(
                          points.length - 1,
                          current + (event.key === "ArrowLeft" ? -1 : 1),
                        ),
                      ),
              );
            }}
          />
        )}
        className="argus-mini-trend"
        aria-label={label}
        onMouseEnter={() => setOpen(true)}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
        onMouseMove={(event) => {
          const rect = event.currentTarget.getBoundingClientRect();
          setSelected(
            Math.round(
              Math.max(
                0,
                Math.min(1, (event.clientX - rect.left) / rect.width),
              ) *
                (points.length - 1),
            ),
          );
        }}
        onMouseLeave={() => {
          setSelected(undefined);
          setOpen(false);
        }}
      >
        <span className="argus-mini-trend__value">{latest}</span>
        <svg viewBox="0 0 120 32" aria-hidden="true" preserveAspectRatio="none">
          {reference !== undefined && (
            <path
              className="argus-mini-trend__reference"
              d={`M2,${y(reference)} L118,${y(reference)}`}
            />
          )}
          <path className="argus-mini-trend__line" d={path} />
          {active && finite(active.value) && selected !== undefined && (
            <circle
              className="argus-mini-trend__point"
              cx={x(Math.min(selected, points.length - 1))}
              cy={y(active.value)}
              r="2"
            />
          )}
        </svg>
      </Tooltip.Trigger>
      <Tooltip.Content className="argus-mini-trend-tooltip">
        {active && renderDetail(active)}
      </Tooltip.Content>
    </Tooltip>
  );
}
