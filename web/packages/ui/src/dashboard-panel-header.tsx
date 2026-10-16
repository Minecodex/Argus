import type { ReactNode } from "react";

export function DashboardPanelHeader({
  title,
  subtitle,
  editing,
  children,
}: {
  title: string;
  subtitle?: string;
  editing?: boolean;
  children?: ReactNode;
}) {
  return (
    <header
      className={`argus-dashboard-panel-header ${editing ? "argus-dashboard-panel-header--editing" : ""}`}
    >
      <div>
        <strong title={title}>{title}</strong>
        {subtitle && <small>{subtitle}</small>}
      </div>
      <div className="argus-dashboard-panel-actions">{children}</div>
    </header>
  );
}
