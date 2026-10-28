import { type HTMLAttributes, type ReactNode } from "react";
import { cx } from "./lib";
import { Card as HeroCard } from "@heroui/react/card";
import { ChevronDown, ChevronRight } from "lucide-react";
import { Button } from "./button";

export function Card({
  className,
  children,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return (
    <HeroCard className={cx("argus-card", className)} {...props}>
      {children}
    </HeroCard>
  );
}

export function CardHeader({
  title,
  description,
  action,
  className,
  onToggle,
  expanded,
  controlsId,
  toggleLabel,
}: {
  title: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
  className?: string;
  onToggle?: () => void;
  expanded?: boolean;
  controlsId?: string;
  toggleLabel?: string;
}) {
  return (
    <header
      className={cx(
        "argus-card__header",
        onToggle && "argus-card__header--collapsible",
        className,
      )}
      onClick={
        onToggle
          ? (event) => {
              const target = event.target as HTMLElement;
              if (!event.currentTarget.contains(target)) return;
              if (
                !target.closest(
                  "button,a,input,select,textarea,[role=button],[role=combobox],[role=checkbox],[role=switch]",
                )
              )
                onToggle();
            }
          : undefined
      }
    >
      <div className="argus-card__heading">
        <div className="argus-card__title">
          {onToggle && (
            <Button
              variant="ghost"
              isIconOnly
              aria-label={toggleLabel}
              aria-expanded={expanded}
              aria-controls={controlsId}
              onPress={onToggle}
            >
              {expanded ? (
                <ChevronDown aria-hidden />
              ) : (
                <ChevronRight aria-hidden />
              )}
            </Button>
          )}
          {title}
        </div>
        {description && (
          <div className="argus-card__description">{description}</div>
        )}
      </div>
      {action && <div className="argus-card__action">{action}</div>}
    </header>
  );
}

export function CardContent({
  className,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return <div className={cx("argus-card__content", className)} {...props} />;
}

export function CardFooter({
  className,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return <footer className={cx("argus-card__footer", className)} {...props} />;
}
