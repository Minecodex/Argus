import {
  Button as HeroButton,
  type ButtonProps as HeroButtonProps,
} from "@heroui/react/button";
import { Spinner } from "@heroui/react/spinner";
import { forwardRef, type AriaRole } from "react";
import { cx } from "./lib";
import { useUiText } from "./locale";

export type ControlSize = "sm" | "md" | "lg";
export type ButtonProps = Omit<HeroButtonProps, "className"> & {
  className?: string;
  /** Content actions (inbox rows, cards) grow with their content. Controls keep fixed sizes. */
  layout?: "control" | "content";
  title?: string;
  role?: AriaRole;
};

/** All portal actions share press, focus, pending and sizing semantics. */
export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  (
    {
      className,
      layout = "control",
      variant = "secondary",
      size = "md",
      isPending,
      isDisabled,
      children,
      type = "button",
      role,
      title,
      render,
      ...props
    },
    ref,
  ) => {
    const text = useUiText();
    return (
      <>
        <HeroButton
          ref={ref}
          className={cx(
            "argus-button",
            `argus-control--${size}`,
            layout === "content" && "argus-button--content",
            className,
          )}
          variant={variant}
          size={size}
          type={type}
          aria-busy={isPending || undefined}
          data-pending={isPending || undefined}
          isDisabled={isDisabled || isPending}
          render={
            render ??
            (role || title
              ? (dom) => <button {...dom} role={role} title={title} />
              : undefined)
          }
          {...props}
        >
          {typeof children === "function" ? (
            children
          ) : (
            <>
              {isPending && (
                <Spinner
                  aria-hidden
                  className="argus-button__spinner"
                  size="sm"
                />
              )}
              {children}
            </>
          )}
        </HeroButton>
        {isPending && (
          <span role="status" className="sr-only">
            {text("正在处理…", "Working…")}
          </span>
        )}
      </>
    );
  },
);
Button.displayName = "Button";
