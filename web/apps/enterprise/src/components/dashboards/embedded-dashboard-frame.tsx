import type { ComponentProps } from "react";
import type { PageShell } from "@argus/ui";
export function EmbeddedDashboardFrame({
  children,
}: ComponentProps<typeof PageShell>) {
  return <section className="argus-dashboard-embedded">{children}</section>;
}
