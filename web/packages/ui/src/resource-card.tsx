import type { ReactNode } from "react";
import { Card } from "./card";
import { Button } from "./button";
import { Dropdown, type DropdownItem } from "./primitives";
import { useUiText } from "./locale";
import { MoreHorizontal } from "lucide-react";
import { cx } from "./lib";
/** Entity identity and navigation stay separate from secondary/destructive actions. */
export function ResourceCard({
  title,
  subtitle,
  status,
  labels,
  facts = [],
  actions,
  menuItems,
  onOpen,
  className,
  children,
}: {
  title: ReactNode;
  subtitle?: ReactNode;
  status?: ReactNode;
  labels?: ReactNode;
  facts?: { label: ReactNode; value: ReactNode }[];
  actions?: ReactNode;
  menuItems?: DropdownItem[];
  onOpen?: () => void;
  className?: string;
  children?: ReactNode;
}) {
  const text = useUiText();
  return (
    <Card className={cx("argus-resource-card", className)}>
      <header className="argus-resource-card__header">
        <div className="argus-resource-card__identity">
          {onOpen ? (
            <h2 className="argus-resource-card__heading">
              <Button
                onPress={onOpen}
                variant="ghost"
                className="argus-resource-card__title"
              >
                {title}
              </Button>
            </h2>
          ) : (
            <h2 className="argus-resource-card__title">{title}</h2>
          )}
          {subtitle && (
            <div className="argus-resource-card__subtitle">{subtitle}</div>
          )}
        </div>
        <div className="argus-resource-card__status">
          {status}
          {!!menuItems?.length && (
            <Dropdown
              items={menuItems}
              trigger={
                <Button
                  isIconOnly
                  variant="ghost"
                  aria-label={text("更多操作", "More actions")}
                >
                  <MoreHorizontal />
                </Button>
              }
            />
          )}
        </div>
      </header>
      {labels && <div className="argus-resource-card__tags">{labels}</div>}
      {facts.length > 0 && (
        <dl className="argus-resource-card__facts">
          {facts.map((fact, index) => (
            <div key={index}>
              <dt>{fact.label}</dt>
              <dd>{fact.value}</dd>
            </div>
          ))}
        </dl>
      )}
      {children}
      {actions && (
        <footer className="argus-resource-card__actions">{actions}</footer>
      )}
    </Card>
  );
}
export function ResourceGrid({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return <div className={cx("argus-resource-grid", className)}>{children}</div>;
}
