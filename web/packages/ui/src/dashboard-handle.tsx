import type { ButtonHTMLAttributes } from "react";
import { cx } from "./lib";
/** Gesture capture is owned by DashboardGrid, rather than a press responder. */
export function DashboardHandle({
  className,
  children,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      {...props}
      type="button"
      className={cx(
        "argus-button argus-control--md button button--ghost button--icon-only",
        className,
      )}
    >
      {children}
    </button>
  );
}
