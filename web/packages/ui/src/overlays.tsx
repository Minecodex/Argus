import { Form } from "@heroui/react/form";
import { useId, type ReactNode } from "react";
import { Button } from "./button";
import { Dialog } from "./primitives";
import { useUiText } from "./locale";
import { DrawerPanel } from "./drawer-panel";

/** Short forms use a content-sized modal; larger editing tasks retain FormDrawer. */
export function FormDialog({
  open,
  onOpenChange,
  title,
  description,
  children,
  submitLabel,
  cancelLabel,
  loading,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children: ReactNode;
  submitLabel?: string;
  cancelLabel?: string;
  loading?: boolean;
  onSubmit: () => void;
}) {
  const text = useUiText(),
    formId = useId();
  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={title}
      description={description}
      dismissable={!loading}
      footer={
        <>
          <Button isDisabled={loading} onPress={() => onOpenChange(false)}>
            {cancelLabel ?? text("取消", "Cancel")}
          </Button>
          <Button
            isPending={loading}
            type="submit"
            form={formId}
            variant="primary"
          >
            {submitLabel ?? text("提交", "Submit")}
          </Button>
        </>
      }
    >
      <Form
        id={formId}
        className="argus-dialog-form"
        validationBehavior="aria"
        onSubmit={(event) => {
          event.preventDefault();
          event.stopPropagation();
          onSubmit();
        }}
      >
        {children}
      </Form>
    </Dialog>
  );
}

export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  danger,
  confirmLabel,
  cancelLabel,
  loading,
  onConfirm,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  danger?: boolean;
  confirmLabel?: string;
  cancelLabel?: string;
  loading?: boolean;
  onConfirm: () => void;
  children?: ReactNode;
}) {
  const text = useUiText();
  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={title}
      description={description}
      className="argus-dialog--confirm"
      footer={
        <>
          <Button isDisabled={loading} onPress={() => onOpenChange(false)}>
            {cancelLabel ?? text("取消", "Cancel")}
          </Button>
          <Button
            isPending={loading}
            onPress={onConfirm}
            variant={danger ? "danger" : "primary"}
          >
            {confirmLabel ?? text("确认", "Confirm")}
          </Button>
        </>
      }
    >
      {children}
    </Dialog>
  );
}
export function FormDrawer({
  open,
  onOpenChange,
  title,
  description,
  children,
  submitLabel,
  cancelLabel,
  loading,
  onSubmit,
  footer,
  width = 480,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children: ReactNode;
  submitLabel?: string;
  cancelLabel?: string;
  loading?: boolean;
  onSubmit?: () => void;
  footer?: ReactNode;
  width?: number;
}) {
  const text = useUiText(),
    formId = useId();
  return (
    <DrawerPanel
      open={open}
      onOpenChange={onOpenChange}
      title={title}
      description={description}
      width={width}
      dismissable={!loading}
      footer={
        footer ?? (
          <>
            <Button isDisabled={loading} onPress={() => onOpenChange(false)}>
              {cancelLabel ?? text("取消", "Cancel")}
            </Button>
            <Button
              isPending={loading}
              type="submit"
              form={formId}
              variant="primary"
            >
              {submitLabel ?? text("提交", "Submit")}
            </Button>
          </>
        )
      }
    >
      <Form
        id={formId}
        aria-label={title}
        className="argus-drawer__fields"
        validationBehavior="aria"
        onSubmit={(event) => {
          event.preventDefault();
          event.stopPropagation();
          onSubmit?.();
        }}
      >
        {children}
      </Form>
    </DrawerPanel>
  );
}
