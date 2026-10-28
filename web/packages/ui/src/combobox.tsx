import { Button } from "./button";
import { ComboBox as HeroComboBox } from "@heroui/react/combo-box";
import { Input as HeroInput } from "@heroui/react/input";
import { ListBox } from "@heroui/react/list-box";
import { Collection, ListBoxLoadMoreItem } from "react-aria-components";
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
  displayLabel?: boolean;
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
  displayLabel = false,
  className,
  fetchOptions,
  contextKey,
  size = "md",
}: ComboBoxProps) {
  const text = useUiText(),
    field = useFieldContext();
  const [search, setSearch] = useState(value),
    [typing, setTyping] = useState(false),
    [items, setItems] = useState(options),
    [cursor, setCursor] = useState<string>(),
    [pending, setPending] = useState(false),
    [error, setError] = useState<"initial" | "more" | null>(null),
    [retry, setRetry] = useState(0);
  const callback = useRef(fetchOptions),
    request = useRef<AbortController | null>(null),
    typedValue = useRef<string | undefined>(undefined);
  const previousValue = useRef(value),
    previousContext = useRef(contextKey);
  const candidateContext = useRef(contextKey);
  const candidateOptions = useRef(options);
  candidateOptions.current = options;
  callback.current = fetchOptions;
  useEffect(() => {
    const changed = previousValue.current !== value;
    const contextChanged = previousContext.current !== contextKey;
    previousValue.current = value;
    previousContext.current = contextKey;
    // A non-custom picker keeps its authoritative selection while the user
    // searches. Candidate pages and metadata refreshes must not erase that text.
    if (!(
      typing &&
      !contextChanged &&
      (!changed || value === typedValue.current)
    )) {
      const selected =
        items.find((item) => item.value === value) ??
        options.find((item) => item.value === value);
      setSearch(
        displayLabel
          ? (selected?.textValue ??
              (typeof selected?.label === "string" ? selected.label : value))
          : value,
      );
      setTyping(false);
    }
  }, [value, items, options, displayLabel, contextKey, typing]);
  useEffect(() => {
    if (!callback.current) setItems(options);
  }, [options]);
  useEffect(() => {
    if (!callback.current || disabled) return;
    if (candidateContext.current !== contextKey) {
      candidateContext.current = contextKey;
      setItems(candidateOptions.current);
    }
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setPending(true);
    setError(null);
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
          if (!controller.signal.aborted) setError("initial");
        })
        .finally(() => {
          if (!controller.signal.aborted) {
            request.current = null;
            setPending(false);
          }
        });
    }, 200);
    return () => {
      clearTimeout(timer);
      controller.abort();
      request.current?.abort();
      request.current = null;
    };
  }, [search, contextKey, disabled, retry]);
  const loadMore = async () => {
    if (!callback.current || !cursor || pending || request.current || disabled)
      return;
    const controller = new AbortController();
    request.current = controller;
    setPending(true);
    setError(null);
    try {
      const page = await callback.current(search, cursor, controller.signal);
      if (!controller.signal.aborted) {
        setItems((old) => [...old, ...page.options]);
        setCursor(page.next_cursor);
      }
    } catch {
      if (!controller.signal.aborted) setError("more");
    } finally {
      if (!controller.signal.aborted) {
        request.current = null;
        setPending(false);
      }
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
      menuTrigger="input"
      allowsEmptyCollection
      className={cx("argus-combobox", `argus-control--${size}`, className)}
      aria-label={ariaLabel}
      aria-labelledby={field?.labelId}
      aria-describedby={mergeAriaIds(field?.descriptionId)}
      isInvalid={field?.invalid}
      isDisabled={disabled}
      allowsCustomValue={allowCustom}
      inputValue={search}
      selectedKey={typing ? null : value || null}
      items={shown}
      defaultFilter={fetchOptions ? () => true : undefined}
      disabledKeys={shown
        .filter((option) => option.disabled)
        .map((option) => option.value)}
      onInputChange={(next) => {
        typedValue.current = next;
        setTyping(true);
        setSearch(next);
        if (allowCustom) onValueChange(next);
      }}
      onSelectionChange={(key) => {
        if (key !== null) {
          typedValue.current = undefined;
          setTyping(false);
          const selected = items.find((item) => item.value === String(key));
          setSearch(
            displayLabel
              ? (selected?.textValue ??
                  (typeof selected?.label === "string"
                    ? selected.label
                    : String(key)))
              : String(key),
          );
          onValueChange(String(key));
        }
      }}
    >
      <HeroComboBox.InputGroup className="argus-combobox__control">
        <HeroInput id={field?.controlId} placeholder={placeholder} />
        <HeroComboBox.Trigger />
      </HeroComboBox.InputGroup>
      <HeroComboBox.Popover className="argus-combobox__popover">
        <ListBox
          className="argus-select__viewport"
          aria-busy={pending}
          renderEmptyState={() =>
            pending
              ? text("正在获取候选…", "Loading options…")
              : text("没有匹配项", "No matches")
          }
        >
          <Collection items={shown}>
            {(option) => (
              <ListBox.Item
                id={option.value}
                textValue={
                  option.textValue ??
                  (typeof option.label === "string"
                    ? option.label
                    : option.value)
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
          </Collection>
          {cursor && (
            <ListBoxLoadMoreItem
              onLoadMore={error ? undefined : loadMore}
              isLoading={pending}
              scrollOffset={0.2}
            >
              <span className="argus-field__hint" role="status">
                {text("正在获取候选…", "Loading options…")}
              </span>
            </ListBoxLoadMoreItem>
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
              onPress={() =>
                error === "more" ? void loadMore() : setRetry((v) => v + 1)
              }
            >
              {text("重试", "Retry")}
            </Button>
          </p>
        )}
      </HeroComboBox.Popover>
    </HeroComboBox>
  );
}
