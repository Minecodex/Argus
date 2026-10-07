import { Popover } from "@heroui/react/popover";
import { X } from "lucide-react";
import type { ReactNode } from "react";
import { Button } from "./button";
import { useUiText } from "./locale";
import { cx } from "./lib";
import { FieldBoundary } from "./form";

/** Anchored controls share focus restoration, scrolling and portal layering. */
export function FilterPopover({
  open,
  onOpenChange,
  trigger,
  title,
  children,
  footer,
  size = "md",
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  trigger: ReactNode;
  title: string;
  children: ReactNode;
  footer?: ReactNode;
  size?: "md" | "lg";
}) {
  const text = useUiText();
  return (
    <Popover isOpen={open} onOpenChange={onOpenChange}>
      {trigger}
      <Popover.Content
        placement="bottom start"
        className={cx(
          "argus-filter-popover",
          size === "lg" && "argus-filter-popover--lg",
        )}
      >
        <Popover.Dialog className="argus-filter-popover__dialog">
          <div className="argus-filter-popover__header">
            <Popover.Heading className="argus-filter-popover__title">
              {title}
            </Popover.Heading>
            <Button
              isIconOnly
              variant="ghost"
              aria-label={text("关闭", "Close")}
              onPress={() => onOpenChange(false)}
            >
              <X aria-hidden />
            </Button>
          </div>
          <div className="argus-filter-popover__body">
            <FieldBoundary>{children}</FieldBoundary>
          </div>
          {footer && (
            <div className="argus-filter-popover__footer">{footer}</div>
          )}
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  );
}
