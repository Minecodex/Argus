import { Popover } from "@heroui/react/popover";
import { X } from "lucide-react";
import type { ReactNode } from "react";
import { Button } from "./button";
import { useUiText } from "./locale";

/** A bounded, scrollable document anchored to its trigger, with managed focus. */
export function ContentPopover({
  open,
  onOpenChange,
  trigger,
  title,
  children,
}: {
  open: boolean;
  onOpenChange(open: boolean): void;
  trigger: ReactNode;
  title: string;
  children: ReactNode;
}) {
  const text = useUiText();
  return (
    <Popover isOpen={open} onOpenChange={onOpenChange}>
      {trigger}
      <Popover.Content placement="bottom end" className="argus-content-popover">
        <Popover.Dialog className="argus-content-popover__dialog">
          <header className="argus-content-popover__header">
            <Popover.Heading>{title}</Popover.Heading>
            <Button
              isIconOnly
              variant="ghost"
              aria-label={text("关闭", "Close")}
              onPress={() => onOpenChange(false)}
            >
              <X aria-hidden />
            </Button>
          </header>
          <div className="argus-content-popover__body">{children}</div>
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  );
}
