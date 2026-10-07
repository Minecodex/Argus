import { Drawer } from "@heroui/react/drawer";
import { X } from "lucide-react";
import type { CSSProperties, ReactNode } from "react";
import { Button } from "./button";
import { FieldBoundary } from "./form";
import { useUiText } from "./locale";

/** Shared supplementary surface for read-only navigation and editing forms. */
export function DrawerPanel({
  open,
  onOpenChange,
  title,
  description,
  children,
  footer,
  width = 480,
  dismissable = true,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children: ReactNode;
  footer?: ReactNode;
  width?: number;
  dismissable?: boolean;
}) {
  const text = useUiText();
  return (
    <Drawer.Backdrop
      isOpen={open}
      onOpenChange={onOpenChange}
      className="argus-dialog__overlay"
      isDismissable={dismissable}
      isKeyboardDismissDisabled={!dismissable}
      style={{ "--argus-drawer-width": `${width}px` } as CSSProperties}
    >
      <Drawer.Content placement="right" className="argus-drawer-content">
        <Drawer.Dialog className="argus-drawer" aria-label={title}>
          <Drawer.Header className="argus-drawer__top">
            <div className="argus-overlay-heading">
              <Drawer.Heading className="argus-dialog__title">
                {title}
              </Drawer.Heading>
              {description && (
                <p className="argus-dialog__description">{description}</p>
              )}
            </div>
            <Button
              slot="close"
              className="argus-overlay-close"
              isIconOnly
              isDisabled={!dismissable}
              aria-label={text("关闭", "Close")}
              variant="ghost"
            >
              <X aria-hidden />
            </Button>
          </Drawer.Header>
          <Drawer.Body className="argus-drawer__body">
            <FieldBoundary>{children}</FieldBoundary>
          </Drawer.Body>
          {footer && (
            <Drawer.Footer className="argus-drawer__footer">
              {footer}
            </Drawer.Footer>
          )}
        </Drawer.Dialog>
      </Drawer.Content>
    </Drawer.Backdrop>
  );
}
