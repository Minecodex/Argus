import {
  createContext,
  forwardRef,
  type InputHTMLAttributes,
  type ReactNode,
  type TextareaHTMLAttributes,
  useContext,
  useId,
} from "react";
import { cx } from "./lib";
import type { ControlSize } from "./button";
import { Input as HeroInput } from "@heroui/react/input";
import { TextArea as HeroTextarea } from "@heroui/react/textarea";
import { Switch as HeroSwitch } from "@heroui/react/switch";

export type FieldRequirement = "required" | "optional" | "none";

export type FieldContextValue = {
  controlId?: string;
  descriptionId?: string;
  invalid: boolean;
  labelId?: string;
  required: boolean;
};

const FieldContext = createContext<FieldContextValue | null>(null);

export function mergeAriaIds(...values: Array<string | undefined>) {
  const result = values
    .flatMap((value) => value?.split(/\s+/) ?? [])
    .filter(Boolean);
  return result.length > 0 ? [...new Set(result)].join(" ") : undefined;
}

export function useFieldContext() {
  return useContext(FieldContext);
}

/** Overlay inputs belong to their own fields, never the trigger's field. */
export function FieldBoundary({ children }: { children: ReactNode }) {
  return <FieldContext.Provider value={null}>{children}</FieldContext.Provider>;
}

export type InputProps = Omit<InputHTMLAttributes<HTMLInputElement>, "size"> & {
  size?: ControlSize;
  appearance?: "field" | "title";
  density?: "default" | "compact";
};
export const Input = forwardRef<HTMLInputElement, InputProps>(
  (
    {
      className,
      size = "md",
      appearance = "field",
      density = "default",
      ...props
    },
    ref,
  ) => {
    const field = useContext(FieldContext);
    return (
      <HeroInput
        ref={ref}
        {...props}
        aria-describedby={mergeAriaIds(
          props["aria-describedby"],
          field?.descriptionId,
        )}
        aria-invalid={props["aria-invalid"] ?? (field?.invalid || undefined)}
        aria-labelledby={mergeAriaIds(
          props["aria-labelledby"],
          field?.controlId ? undefined : field?.labelId,
        )}
        aria-required={props["aria-required"] ?? (field?.required || undefined)}
        className={cx(
          "argus-input",
          `argus-control--${size}`,
          density === "compact" && "argus-input--compact",
          appearance === "title" && "argus-input--title",
          className,
        )}
        id={props.id ?? field?.controlId}
        required={props.required ?? field?.required}
      />
    );
  },
);
Input.displayName = "Input";

export type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement> & {
  /** The enclosing composite control supplies the border and focus indicator. */
  appearance?: "field" | "embedded";
};

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, appearance = "field", ...props }, ref) => {
    const field = useContext(FieldContext);
    return (
      <HeroTextarea
        ref={ref}
        {...props}
        aria-describedby={mergeAriaIds(
          props["aria-describedby"],
          field?.descriptionId,
        )}
        aria-invalid={props["aria-invalid"] ?? (field?.invalid || undefined)}
        aria-labelledby={mergeAriaIds(
          props["aria-labelledby"],
          field?.controlId ? undefined : field?.labelId,
        )}
        aria-required={props["aria-required"] ?? (field?.required || undefined)}
        className={cx(
          "argus-textarea",
          appearance === "embedded" && "argus-textarea--embedded",
          className,
        )}
        id={props.id ?? field?.controlId}
        required={props.required ?? field?.required}
      />
    );
  },
);
Textarea.displayName = "Textarea";

export function Field({
  label,
  hint,
  error,
  children,
  className,
  controlId: providedControlId,
  controlMode = "single",
  requirement,
  density = "default",
}: {
  label: string;
  requirement: FieldRequirement;
  hint?: string;
  error?: string;
  children: ReactNode;
  className?: string;
  controlId?: string;
  controlMode?: "single" | "group";
  density?: "default" | "compact";
}) {
  const generatedId = useId();
  const controlId =
    controlMode === "single"
      ? (providedControlId ?? `${generatedId}-control`)
      : undefined;
  const labelId = `${generatedId}-label`;
  const messageId = `${generatedId}-message`;
  const required = requirement === "required";
  const context: FieldContextValue = {
    controlId,
    descriptionId: hint || error ? messageId : undefined,
    invalid: Boolean(error),
    labelId: controlMode === "single" ? labelId : undefined,
    required,
  };
  return (
    <div
      aria-labelledby={controlMode === "group" ? labelId : undefined}
      className={cx(
        "argus-field",
        density === "compact" && "argus-field--compact",
        className,
      )}
      role={controlMode === "group" ? "group" : undefined}
    >
      <label className="argus-field__label" htmlFor={controlId} id={labelId}>
        {label}
        {required && (
          <span aria-hidden className="argus-field__required">
            *
          </span>
        )}
      </label>
      <FieldContext.Provider value={context}>{children}</FieldContext.Provider>
      {(hint || error) && (
        <span
          className={cx("argus-field__hint", error && "is-error")}
          id={messageId}
        >
          {error || hint}
        </span>
      )}
    </div>
  );
}

export function Switch({
  checked,
  onChange,
  label,
  disabled = false,
  descriptionId,
}: {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label: string;
  disabled?: boolean;
  descriptionId?: string;
}) {
  return (
    <HeroSwitch
      aria-label={label}
      aria-describedby={descriptionId}
      className="argus-switch"
      isSelected={checked}
      isDisabled={disabled}
      onChange={onChange}
    >
      <HeroSwitch.Content aria-label={label}>
        <HeroSwitch.Control>
          <HeroSwitch.Thumb />
        </HeroSwitch.Control>
        <span className="sr-only">{label}</span>
      </HeroSwitch.Content>
    </HeroSwitch>
  );
}
