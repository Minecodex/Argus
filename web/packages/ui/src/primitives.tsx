import { Avatar as HeroAvatar } from "@heroui/react/avatar";
import { Tabs as HeroTabs } from "@heroui/react/tabs";
import { Tooltip as HeroTooltip } from "@heroui/react/tooltip";
import { Dropdown as HeroDropdown } from "@heroui/react/dropdown";
import { FieldBoundary } from "./form";
import { Modal } from "@heroui/react/modal";
import { Label } from "@heroui/react/label";
import { Separator } from "@heroui/react/separator";
import { Check, ChevronRight, X } from "lucide-react";
import {
  cloneElement,
  isValidElement,
  type ReactNode,
  type ReactElement,
  type CSSProperties,
  type HTMLAttributes,
} from "react";
import { cx } from "./lib";
import { useUiText } from "./locale";
import { Button } from "./button";

export function Avatar({
  fallback,
  src,
  size = "md",
}: {
  fallback: string;
  src?: string;
  size?: "sm" | "md" | "lg";
}) {
  return (
    <HeroAvatar
      aria-label={fallback}
      className={cx("argus-avatar", `argus-avatar--${size}`)}
      size={size}
    >
      {src && <HeroAvatar.Image alt="" src={src} />}
      <HeroAvatar.Fallback>{fallback}</HeroAvatar.Fallback>
    </HeroAvatar>
  );
}
export function Tabs({
  children,
  value,
  defaultValue,
  onValueChange,
  className,
}: {
  children: ReactNode;
  value?: string;
  defaultValue?: string;
  onValueChange?: (value: string) => void;
  className?: string;
}) {
  return (
    <HeroTabs
      variant="secondary"
      className={className}
      selectedKey={value}
      defaultSelectedKey={defaultValue}
      onSelectionChange={(key) => onValueChange?.(String(key))}
    >
      {children}
    </HeroTabs>
  );
}
export function TabsList({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  const text = useUiText();
  return (
    <HeroTabs.ListContainer>
      <HeroTabs.List
        aria-label={text("页面分区", "Page sections")}
        className={cx("argus-tabs", className)}
      >
        {children}
      </HeroTabs.List>
    </HeroTabs.ListContainer>
  );
}
export function TabsTrigger({
  value,
  children,
}: {
  value: string;
  children: ReactNode;
}) {
  return (
    <HeroTabs.Tab id={value} className="argus-tabs__trigger">
      {children}
      <HeroTabs.Indicator />
    </HeroTabs.Tab>
  );
}
export function TabsContent({
  value,
  children,
  className,
}: {
  value: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <HeroTabs.Panel id={value} className={cx("argus-tabs__content", className)}>
      {children}
    </HeroTabs.Panel>
  );
}
export function Tooltip({
  children,
  content,
}: {
  children: ReactNode;
  content: ReactNode;
}) {
  const trigger = isValidElement(children) ? (
    children
  ) : (
    <Button>{children}</Button>
  );
  const element = trigger as ReactElement<HTMLAttributes<HTMLDivElement>>;
  return (
    <HeroTooltip delay={350}>
      <HeroTooltip.Trigger
        role={undefined}
        render={(props) =>
          cloneElement(
            element,
            {
              ...props,
              className: cx(element.props.className, props.className),
            },
            element.props.children,
          )
        }
      />
      <HeroTooltip.Content className="argus-tooltip">
        <HeroTooltip.Arrow />
        {content}
      </HeroTooltip.Content>
    </HeroTooltip>
  );
}
export function Dialog({
  trigger,
  title,
  description,
  children,
  footer,
  open,
  onOpenChange,
  size = "md",
  width,
  className,
  dismissable = true,
}: {
  trigger?: ReactNode;
  title: string;
  description?: string;
  children: ReactNode;
  footer?: ReactNode;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  size?: "md" | "lg";
  width?: number;
  className?: string;
  dismissable?: boolean;
}) {
  const text = useUiText();
  const body = (
    <Modal.Backdrop
      isOpen={trigger ? undefined : open}
      onOpenChange={trigger ? undefined : onOpenChange}
      className="argus-dialog__overlay"
      isDismissable={dismissable}
      isKeyboardDismissDisabled={!dismissable}
      style={
        width
          ? ({ "--argus-dialog-width": `${width}px` } as CSSProperties)
          : undefined
      }
    >
      <Modal.Container size={size} className="argus-dialog-container">
        <Modal.Dialog
          className={cx(
            "argus-dialog",
            size === "lg" && "argus-dialog--lg",
            className,
          )}
        >
          <Modal.Header className="argus-dialog__top">
            <div className="argus-overlay-heading">
              <Modal.Heading className="argus-dialog__title">
                {title}
              </Modal.Heading>
              {description && (
                <p className="argus-dialog__description">{description}</p>
              )}
            </div>
            <Button
              slot="close"
              className="argus-overlay-close"
              isDisabled={!dismissable}
              isIconOnly
              variant="ghost"
              aria-label={text("关闭", "Close")}
            >
              <X aria-hidden />
            </Button>
          </Modal.Header>
          <Modal.Body className="argus-dialog__body">
            <FieldBoundary>{children}</FieldBoundary>
          </Modal.Body>
          {footer && (
            <Modal.Footer className="argus-dialog__footer">
              {footer}
            </Modal.Footer>
          )}
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
  return trigger ? (
    <Modal isOpen={open} onOpenChange={onOpenChange}>
      {trigger}
      {body}
    </Modal>
  ) : (
    body
  );
}
export type DropdownItem =
  | {
      label: string;
      shortcut?: string;
      danger?: boolean;
      onSelect?: () => void;
    }
  | "separator";
export function Dropdown({
  trigger,
  items,
}: {
  trigger: ReactNode;
  items: DropdownItem[];
}) {
  const text = useUiText();
  return (
    <HeroDropdown>
      {trigger}
      <HeroDropdown.Popover className="argus-dropdown" placement="bottom end">
        <HeroDropdown.Menu
          aria-label={text("操作", "Actions")}
          onAction={(key) => {
            const item = items[Number(key)];
            if (item && item !== "separator") item.onSelect?.();
          }}
        >
          {items.map((item, index) =>
            item === "separator" ? (
              <Separator key={index} />
            ) : (
              <HeroDropdown.Item
                id={String(index)}
                key={index}
                textValue={item.label}
                className={cx(
                  "argus-dropdown__item",
                  item.danger && "is-danger",
                )}
                variant={item.danger ? "danger" : undefined}
              >
                <Label>{item.label}</Label>
                {item.shortcut && <kbd>{item.shortcut}</kbd>}
              </HeroDropdown.Item>
            ),
          )}
        </HeroDropdown.Menu>
      </HeroDropdown.Popover>
    </HeroDropdown>
  );
}
export function MenuItem({
  active,
  children,
  end,
}: {
  active?: boolean;
  children: ReactNode;
  end?: ReactNode;
}) {
  return (
    <div className={cx("argus-menu-item", active && "is-active")}>
      <span>{children}</span>
      {end ?? <ChevronRight size={14} />}
    </div>
  );
}
export function CheckItem({
  checked,
  children,
}: {
  checked: boolean;
  children: ReactNode;
}) {
  return (
    <div className="argus-check-item">
      <span className={cx("argus-check-item__box", checked && "is-checked")}>
        {checked && <Check size={12} />}
      </span>
      {children}
    </div>
  );
}
