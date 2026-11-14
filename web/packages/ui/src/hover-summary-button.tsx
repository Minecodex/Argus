import { useEffect, useRef, useState } from "react";
import { Popover } from "@heroui/react/popover";
import { Copy, Check } from "lucide-react";
import { Button } from "./button";
import { useUiText } from "./locale";

/** An inspectable summary; activating the name preserves its primary action. */
export function HoverSummaryButton({
  label,
  fields,
  onPress,
  className,
  isSummaryDisabled = false,
}: {
  label: string;
  fields: { label: string; value: string; copyable?: boolean }[];
  onPress: () => void;
  className?: string;
  isSummaryDisabled?: boolean;
}) {
  const [open, setOpen] = useState(false),
    [copied, setCopied] = useState<number>(),
    [failed, setFailed] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined),
    text = useUiText();
  const enter = () => {
    clearTimeout(timer.current);
    if (!isSummaryDisabled) setOpen(true);
  };
  const leave = () => {
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setOpen(false), 200);
  };
  useEffect(() => () => clearTimeout(timer.current), []);
  useEffect(() => {
    if (isSummaryDisabled) {
      clearTimeout(timer.current);
      setOpen(false);
    }
  }, [isSummaryDisabled]);
  if (!fields.length)
    return (
      <Button
        variant="ghost"
        className={className}
        title={label}
        onPress={onPress}
      >
        <span className="argus-hover-summary-name">{label}</span>
      </Button>
    );
  return (
    <Popover
      isOpen={open && !isSummaryDisabled}
      onOpenChange={(value) => {
        if (!value) setOpen(false);
      }}
    >
      <Button
        variant="ghost"
        className={className}
        onHoverStart={enter}
        onHoverEnd={leave}
        onFocus={(event) => {
          if (event.currentTarget.matches(":focus-visible")) enter();
        }}
        onBlur={leave}
        onPress={() => {
          clearTimeout(timer.current);
          setOpen(false);
          onPress();
        }}
      >
        <span className="argus-hover-summary-name">{label}</span>
      </Button>
      <Popover.Content
        isNonModal
        placement="bottom start"
        className="argus-hover-summary"
        onMouseEnter={enter}
        onMouseLeave={leave}
      >
        <Popover.Dialog>
          <div onFocusCapture={enter} onBlurCapture={leave}>
            <Popover.Heading>{label}</Popover.Heading>
            <dl className="argus-hover-summary__fields">
              {fields.map((field, index) => (
                <div key={`${field.label}:${index}`}>
                  <dt>{field.label}</dt>
                  <dd>
                    {field.value}
                    {field.copyable && (
                      <Button
                        isIconOnly
                        size="sm"
                        variant="ghost"
                        aria-label={`${text("复制", "Copy")} ${field.label}`}
                        onPress={async () => {
                          try {
                            await navigator.clipboard.writeText(field.value);
                            setCopied(index);
                            setFailed(false);
                          } catch {
                            setFailed(true);
                          }
                        }}
                      >
                        {copied === index ? (
                          <Check aria-hidden size={14} />
                        ) : (
                          <Copy aria-hidden size={14} />
                        )}
                      </Button>
                    )}
                  </dd>
                </div>
              ))}
            </dl>
            {failed && (
              <p role="status">
                {text(
                  "复制失败，请选择文本复制。",
                  "Copy failed. Select the text to copy it.",
                )}
              </p>
            )}
          </div>
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  );
}
