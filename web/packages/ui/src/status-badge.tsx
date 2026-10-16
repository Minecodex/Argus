import { type HTMLAttributes } from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cx } from "./lib";

const statusBadgeVariants = cva("argus-status-badge", {
  variants: {
    tone: {
      neutral: "argus-status-badge--neutral",
      accent: "argus-status-badge--accent",
      success: "argus-status-badge--success",
      warning: "argus-status-badge--warning",
      danger: "argus-status-badge--danger",
      info: "argus-status-badge--info",
    },
    pulse: { true: "argus-status-badge--pulse" },
    appearance: { flat: "argus-status-badge--flat" },
  },
  defaultVariants: { tone: "neutral" },
});

export type StatusBadgeProps = HTMLAttributes<HTMLSpanElement> &
  VariantProps<typeof statusBadgeVariants>;

export function StatusBadge({
  className,
  tone,
  pulse,
  appearance,
  children,
  ...props
}: StatusBadgeProps) {
  return (
    <span
      className={cx(
        statusBadgeVariants({ tone, pulse, appearance }),
        className,
      )}
      {...props}
    >
      <i aria-hidden className="argus-status-badge__dot" />
      {children}
    </span>
  );
}
