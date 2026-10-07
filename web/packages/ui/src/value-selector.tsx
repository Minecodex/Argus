import { Checkbox } from "./checkbox";
import { RadioOptions } from "./radio-options";
import { useEffect, useId, useRef, useState } from "react";
import { Button } from "./button";
import { Dialog } from "./primitives";
import { Input } from "./form";
import { FilterPopover } from "./filter-popover";
import { ChevronDown } from "lucide-react";
import { useUiText } from "./locale";
import {
  selectionDisappeared,
  type CandidatePage,
} from "./value-selection-proof";
export type ValueSelection = { all: boolean; values: string[] };
export function ValueSelector({
  label,
  value,
  multiple = false,
  candidates,
  onChange,
  fetchValues,
  disabled,
  contextKey,
  valueLabels,
  allLabel,
  presentation = "dialog",
  title,
  hint,
}: {
  label: string;
  value: ValueSelection;
  multiple?: boolean;
  candidates: string[];
  onChange: (value: ValueSelection) => void;
  fetchValues?: (
    search: string,
    cursor: string | undefined,
    signal: AbortSignal,
  ) => Promise<CandidatePage>;
  disabled?: boolean;
  /** Change when the query scope or its dependencies change. */
  contextKey?: string;
  valueLabels?: Record<string, string>;
  allLabel?: string;
  presentation?: "dialog" | "popover";
  title?: string;
  hint?: string;
}) {
  const text = useUiText(),
    name = useId();
  const [open, setOpen] = useState(false),
    [selection, setSelection] = useState(value),
    [search, setSearch] = useState(""),
    [options, setOptions] = useState(candidates),
    [cursor, setCursor] = useState<string>(),
    [loading, setLoading] = useState(false),
    [error, setError] = useState(false),
    [retry, setRetry] = useState(0);
  const callback = useRef(fetchValues),
    request = useRef<AbortController | undefined>(undefined);
  const currentValue = useRef(value);
  const change = useRef(onChange);
  currentValue.current = value;
  change.current = onChange;
  callback.current = fetchValues;
  // Static directories may finish loading after the popup opens. Keep them
  // live; only a remote paged lookup owns its separate options snapshot.
  const available = fetchValues ? options : candidates;
  const load = async (search: string, cursor?: string) => {
    if (!callback.current) return;
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setLoading(true);
    setError(false);
    const captured = currentValue.current;
    try {
      const page = await callback.current(search, cursor, controller.signal);
      if (!controller.signal.aborted) {
        setOptions((previous) =>
          cursor ? [...previous, ...page.values] : page.values,
        );
        setCursor(page.next_cursor);
        // Only explicit existence evidence (or a complete unfiltered result)
        // can reset a saved selection. A page, failed lookup or stale request cannot.
        if (
          !captured.all &&
          JSON.stringify(captured) === JSON.stringify(currentValue.current) &&
          selectionDisappeared(captured.values, page)
        ) {
          const reconciled = { all: true, values: [] };
          setSelection(reconciled);
          change.current(reconciled);
        }
      }
    } catch {
      if (!controller.signal.aborted) setError(true);
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  };
  useEffect(() => {
    // An upstream change can reconcile the effective selection while this
    // dialog is open. Do not later reapply the old draft selection.
    setSelection(currentValue.current);
  }, [contextKey]);
  useEffect(() => {
    if (!open || !callback.current) return;
    request.current?.abort();
    setLoading(true);
    setCursor(undefined);
    setOptions([]);
    if (disabled) return;
    const timer = setTimeout(() => {
      void load(search);
    }, 200);
    return () => {
      clearTimeout(timer);
      request.current?.abort();
    };
  }, [open, search, retry, contextKey, disabled]);
  const trigger = (
    <Button
      isDisabled={disabled}
      aria-label={label}
      aria-describedby={`${name}-selection`}
      onPress={() => {
        setSelection(value);
        setOptions(candidates);
        setSearch("");
        setError(false);
        setLoading(Boolean(callback.current));
        setCursor(undefined);
        setOpen(true);
      }}
    >
      <span id={`${name}-selection`} className="argus-value-selector-summary">
        {label}:{" "}
        {value.all
          ? (allLabel ?? text("全部", "All"))
          : value.values.map((v) => valueLabels?.[v] ?? v).join(", ")}
      </span>
      {presentation === "popover" && <ChevronDown aria-hidden />}
    </Button>
  );
  const footer = (
    <>
      <Button onPress={() => setOpen(false)}>{text("取消", "Cancel")}</Button>
      <Button
        variant="primary"
        isDisabled={disabled || (!selection.all && !selection.values.length)}
        onPress={() => {
          onChange(selection);
          setOpen(false);
        }}
      >
        {text("应用", "Apply")}
      </Button>
    </>
  );
  const contents = (
    <div className="argus-value-selector">
      {hint && <p className="argus-value-selector-hint">{hint}</p>}
      <Input
        aria-label={text("搜索候选值", "Search candidates")}
        value={search}
        onChange={(e) => setSearch(e.target.value)}
      />
      <Checkbox
        isSelected={selection.all}
        onChange={(selected) => setSelection({ all: selected, values: [] })}
      >
        {allLabel ?? text("全部", "All")}
      </Checkbox>
      {multiple ? (
        Array.from(new Set([...selection.values, ...available]))
          .filter(
            (v) =>
              fetchValues ||
              `${valueLabels?.[v] ?? ""} ${v}`
                .toLowerCase()
                .includes(search.toLowerCase()),
          )
          .map((v) => (
            <Checkbox
              key={v}
              isDisabled={disabled}
              isSelected={!selection.all && selection.values.includes(v)}
              onChange={(nextChecked) =>
                setSelection({
                  all: false,
                  values: nextChecked
                    ? [...selection.values.filter((p) => p !== v), v]
                    : selection.values.filter((p) => p !== v),
                })
              }
            >
              {valueLabels?.[v] ?? v}
            </Checkbox>
          ))
      ) : (
        <RadioOptions
          label={label}
          disabled={disabled}
          value={selection.all ? "" : (selection.values[0] ?? "")}
          options={Array.from(new Set([...selection.values, ...available]))
            .filter(
              (v) =>
                fetchValues ||
                `${valueLabels?.[v] ?? ""} ${v}`
                  .toLowerCase()
                  .includes(search.toLowerCase()),
            )
            .map((v) => ({ value: v, label: valueLabels?.[v] ?? v }))}
          onChange={(v) => setSelection({ all: false, values: [v] })}
        />
      )}
      {error && (
        <>
          <p role="alert">
            {text(
              "候选查询失败，当前选值保持不变。",
              "Candidate lookup failed. Your selection is unchanged.",
            )}
          </p>
          <Button onPress={() => setRetry(retry + 1)}>
            {text("重试", "Retry")}
          </Button>
        </>
      )}
      {cursor && (
        <Button isDisabled={loading} onPress={() => void load(search, cursor)}>
          {text("更多候选", "More candidates")}
        </Button>
      )}
      {loading && <p role="status">{text("正在查询…", "Loading…")}</p>}
      {!loading && !error && !available.length && (
        <p className="argus-value-selector-hint">
          {text("没有可用候选值", "No candidates available")}
        </p>
      )}
    </div>
  );
  return presentation === "popover" ? (
    <FilterPopover
      open={open}
      onOpenChange={setOpen}
      trigger={trigger}
      title={title ?? label}
      footer={footer}
    >
      {contents}
    </FilterPopover>
  ) : (
    <>
      {trigger}
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={title ?? label}
        footer={footer}
      >
        {contents}
      </Dialog>
    </>
  );
}
