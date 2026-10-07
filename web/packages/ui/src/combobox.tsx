import { Button } from "./button";
import { ComboBox as HeroComboBox } from "@heroui/react/combo-box";
import { Input as HeroInput } from "@heroui/react/input";
import { ListBox } from "@heroui/react/list-box";
import { Label } from "@heroui/react/label";
import { Description } from "@heroui/react/description";
import { useEffect, useRef, useState } from "react";
import { cx } from "./lib";
import { mergeAriaIds, useFieldContext } from "./form";
import { useUiText } from "./locale";
import type { SelectOption } from "./select";
import type { ControlSize } from "./button";

export type ComboBoxProps = {
  value: string;
  onValueChange: (value: string) => void;
  options: SelectOption[];
  ariaLabel?: string;
  placeholder?: string;
  disabled?: boolean;
  allowCustom?: boolean;
  className?: string;
  fetchOptions?: (
    search: string,
    cursor: string | undefined,
    signal: AbortSignal,
  ) => Promise<{ options: SelectOption[]; next_cursor?: string }>;
  contextKey?: string;
  size?: ControlSize;
};
/** Remote candidates never reconcile the authoritative value from one page. */
export function ComboBox({
  value,
  onValueChange,
  options,
  ariaLabel,
  placeholder,
  disabled,
  allowCustom,
  className,
  fetchOptions,
  contextKey,
  size = "md",
}: ComboBoxProps) {
  const text = useUiText(),
    field = useFieldContext();
  const [search, setSearch] = useState(value),
    [items, setItems] = useState(options),
    [cursor, setCursor] = useState<string>(),
    [pending, setPending] = useState(false),
    [error, setError] = useState(false),
    [retry, setRetry] = useState(0);
  const callback = useRef(fetchOptions),
    request = useRef<AbortController | null>(null);
  callback.current = fetchOptions;
  useEffect(() => setSearch(value), [value]);
  useEffect(() => {
    if (!callback.current) setItems(options);
  }, [options]);
  useEffect(() => {
    if (!callback.current || disabled) return;
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setPending(true);
    setError(false);
    setCursor(undefined);
    const timer = setTimeout(() => {
      void callback.current!(search, undefined, controller.signal)
        .then((page) => {
          if (!controller.signal.aborted) {
            setItems(page.options);
            setCursor(page.next_cursor);
          }
        })
        .catch(() => {
          if (!controller.signal.aborted) setError(true);
        })
        .finally(() => {
          if (!controller.signal.aborted) setPending(false);
        });
    }, 200);
    return () => {
      clearTimeout(timer);
      request.current?.abort();
    };
  }, [search, contextKey, disabled, retry]);
  const loadMore = async () => {
    if (!callback.current || !cursor || pending) return;
    const controller = new AbortController();
    request.current?.abort();
    request.current = controller;
    setPending(true);
    try {
      const page = await callback.current(search, cursor, controller.signal);
      if (!controller.signal.aborted) {
        setItems((old) => [...old, ...page.options]);
        setCursor(page.next_cursor);
      }
    } catch {
      if (!controller.signal.aborted) setError(true);
    } finally {
      if (!controller.signal.aborted) setPending(false);
    }
  };
  const current = items.find((option) => option.value === value),
    seen = new Set<string>();
  const shown = [
    ...(current || !value ? [] : [{ value, label: value }]),
    ...items,
  ].filter(
    (option) => !seen.has(option.value) && Boolean(seen.add(option.value)),
  );
  return (
    <HeroComboBox
      className={cx("argus-combobox", `argus-control--${size}`, className)}
      aria-label={ariaLabel}
      aria-labelledby={field?.labelId}
      aria-describedby={mergeAriaIds(field?.descriptionId)}
      isInvalid={field?.invalid}
      isDisabled={disabled}
      allowsCustomValue={allowCustom}
      inputValue={search}
      selectedKey={value || null}
      items={shown}
      defaultFilter={fetchOptions ? () => true : undefined}
      disabledKeys={shown
        .filter((option) => option.disabled)
        .map((option) => option.value)}
      onInputChange={(next) => {
        setSearch(next);
        if (allowCustom) onValueChange(next);
      }}
      onSelectionChange={(key) => {
        if (key !== null) onValueChange(String(key));
      }}
    >
      <HeroComboBox.InputGroup className="argus-combobox__control">
        <HeroInput id={field?.controlId} placeholder={placeholder} />
        <HeroComboBox.Trigger />
      </HeroComboBox.InputGroup>
      <HeroComboBox.Popover className="argus-combobox__popover">
        <ListBox
          items={shown}
          className="argus-select__viewport"
          renderEmptyState={() =>
            pending
              ? text("正在获取候选…", "Loading options…")
              : text("没有匹配项", "No matches")
          }
        >
          {(option) => (
            <ListBox.Item
              id={option.value}
              textValue={
                option.textValue ??
                (typeof option.label === "string" ? option.label : option.value)
              }
              className="argus-select__item"
            >
              <div>
                <Label>{option.label}</Label>
                {option.description && (
                  <Description>{option.description}</Description>
                )}
              </div>
              <ListBox.ItemIndicator />
            </ListBox.Item>
          )}
        </ListBox>
        {error && (
          <p className="argus-field__hint is-error" role="alert">
            {text(
              "候选获取失败，当前选值保持不变。",
              "Options could not be loaded. Your selection is unchanged.",
            )}
            <Button
              variant="ghost"
              type="button"
              onPress={() => setRetry((v) => v + 1)}
            >
              {text("重试", "Retry")}
            </Button>
          </p>
        )}
        {cursor && (
          <Button
            variant="ghost"
            type="button"
            isDisabled={pending}
            onPress={() => void loadMore()}
          >
            {text("加载更多", "Load more")}
          </Button>
        )}
      </HeroComboBox.Popover>
    </HeroComboBox>
  );
}
