import type { ReactNode } from "react";
import { Popover } from "@heroui/react";
import { CircleAlert } from "lucide-react";
import { Button } from "./button";

/** A keyboard-accessible, on-demand explanation beside a heading. */
export function InfoPopover({
  title,
  label = title,
  attention,
  children,
}: {
  title: string;
  label?: string;
  attention?: boolean;
  children: ReactNode;
}) {
  return (
    <Popover>
      <Button
        isIconOnly
        size="sm"
        variant="ghost"
        aria-label={label}
        className="argus-info-trigger"
        data-attention={attention || undefined}
      >
        <CircleAlert size={16} />
      </Button>
      <Popover.Content className="argus-info-popover" placement="bottom start">
        <Popover.Dialog>
          <Popover.Heading>{title}</Popover.Heading>
          <div className="argus-info-content">{children}</div>
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  );
}
