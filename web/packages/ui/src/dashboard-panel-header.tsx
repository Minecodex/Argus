import type { ReactNode } from "react";

export function DashboardPanelHeader({
  title,
  subtitle,
  info,
  editing,
  children,
}: {
  title: string;
  subtitle?: string;
  info?: ReactNode;
  editing?: boolean;
  children?: ReactNode;
}) {
  return (
    <header
      className={`argus-dashboard-panel-header ${editing ? "argus-dashboard-panel-header--editing" : ""}`}
    >
      <div>
        <div className="argus-dashboard-panel-title">
          <strong title={title}>{title}</strong>
          {info}
        </div>
        {subtitle && <small>{subtitle}</small>}
      </div>
      <div className="argus-dashboard-panel-actions">{children}</div>
    </header>
  );
}
