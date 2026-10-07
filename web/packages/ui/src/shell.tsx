import type { CSSProperties, ReactNode, Ref } from "react";
import { ChevronDown, ChevronsLeft, ChevronsRight } from "lucide-react";
import { useUiText } from "./locale";
import { Avatar, Dropdown, Tooltip } from "./primitives";
import { Button, type ButtonProps } from "./button";
import { cx } from "./lib";

export type UserMenuItem =
  | {
      label: string;
      shortcut?: string;
      danger?: boolean;
      onSelect?: () => void;
    }
  | "separator";

export function SidebarToggle({
  collapsed,
  onToggle,
}: {
  collapsed: boolean;
  onToggle: () => void;
}) {
  const text = useUiText();
  return (
    <Button
      isIconOnly
      variant="ghost"
      className="argus-sidebar__collapse"
      aria-label={
        collapsed
          ? text("展开导航", "Expand navigation")
          : text("收起导航", "Collapse navigation")
      }
      onPress={onToggle}
    >
      {collapsed ? <ChevronsRight size={16} /> : <ChevronsLeft size={16} />}
    </Button>
  );
}

/** Where a persistent dock panel attaches to the shell grid. */
export type AppShellDockPlacement = {
  position: "bottom" | "left" | "right";
  /** Dock size as a percentage of the main column's matching dimension. */
  sizePercent: number;
};

export function AppShell({
  sidebar,
  header,
  children,
  overlay,
  dock,
  className,
  style,
  mainRef,
  dockPlacement,
}: {
  sidebar: ReactNode;
  header: ReactNode;
  children: ReactNode;
  overlay?: ReactNode;
  dock?: ReactNode;
  className?: string;
  style?: CSSProperties;
  /** Ref to the page scroll container; the shell itself never scrolls. */
  mainRef?: Ref<HTMLElement>;
  /** When set, the dock joins the shell grid at this placement. */
  dockPlacement?: AppShellDockPlacement;
}) {
  const rootStyle: CSSProperties | undefined = dockPlacement
    ? ({
        ...style,
        "--terminal-dock-size": `${dockPlacement.sizePercent}%`,
      } as CSSProperties)
    : style;
  return (
    <div
      className={cx("argus-app-shell", className)}
      data-terminal-dock={dockPlacement?.position}
      style={rootStyle}
    >
      {sidebar}
      <div className="argus-app-main">
        {header}
        <main className="argus-page-content" ref={mainRef} tabIndex={0}>
          <div className="argus-app-workspace">{children}</div>
        </main>
      </div>
      {overlay}
      {dock}
    </div>
  );
}

export function PortalUserMenu({
  displayName,
  username,
  items,
}: {
  displayName: string;
  username: string;
  items: UserMenuItem[];
}) {
  return (
    <Dropdown
      items={items}
      trigger={
        <Button
          variant="ghost"
          className="argus-user-menu"
          type="button"
          aria-label={`${displayName} ${username}`}
        >
          <Avatar fallback={displayName.slice(0, 1)} />
          <span>
            <b>{displayName}</b>
            <small>{username}</small>
          </span>
          <ChevronDown aria-hidden size={13} />
        </Button>
      }
    />
  );
}

export function IconButton({
  label,
  children,
  ...props
}: Omit<ButtonProps, "aria-label" | "size"> & {
  label: string;
  children: ReactNode;
}) {
  return (
    <Tooltip content={label}>
      <Button aria-label={label} isIconOnly {...props}>
        {children}
      </Button>
    </Tooltip>
  );
}

export function AuthStatePage({
  status,
  title,
  message,
}: {
  status: "checking" | "unavailable";
  title?: string;
  message?: string | null;
}) {
  if (status === "checking") {
    return <main aria-busy="true" className="argus-auth-state" />;
  }
  return (
    <main className="argus-auth-state" role="alert">
      {title && <h1>{title}</h1>}
      {message && <p>{message}</p>}
    </main>
  );
}
