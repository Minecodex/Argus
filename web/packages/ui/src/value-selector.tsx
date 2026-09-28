import { useEffect, useId, useRef, useState } from "react";
import { Button } from "./button";
import { Dialog } from "./primitives";
import { Input } from "./form";
import { useUiText } from "./locale";
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
  ) => Promise<{ values: string[]; next_cursor?: string }>;
  disabled?: boolean;
  /** Change when the query scope or its dependencies change. */
  contextKey?: string;
  valueLabels?: Record<string, string>;
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
  currentValue.current = value;
  callback.current = fetchValues;
  const load = async (search: string, cursor?: string) => {
    if (!callback.current) return;
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setLoading(true);
    setError(false);
    try {
      const page = await callback.current(search, cursor, controller.signal);
      if (!controller.signal.aborted) {
        setOptions((previous) =>
          cursor ? [...previous, ...page.values] : page.values,
        );
        setCursor(page.next_cursor);
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
  return (
    <>
      <Button
        disabled={disabled}
        aria-label={label}
        aria-describedby={`${name}-selection`}
        onClick={() => {
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
            ? text("全部", "All")
            : value.values.map((v) => valueLabels?.[v] ?? v).join(", ")}
        </span>
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={label}
        footer={
          <>
            <Button onClick={() => setOpen(false)}>
              {text("取消", "Cancel")}
            </Button>
            <Button
              variant="primary"
              disabled={
                disabled || (!selection.all && !selection.values.length)
              }
              onClick={() => {
                onChange(selection);
                setOpen(false);
              }}
            >
              {text("应用", "Apply")}
            </Button>
          </>
        }
      >
        <div className="argus-value-selector">
          <Input
            aria-label={text("搜索候选值", "Search candidates")}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <label>
            <input
              type="checkbox"
              checked={selection.all}
              onChange={(e) =>
                setSelection({ all: e.target.checked, values: [] })
              }
            />
            {text("全部", "All")}
          </label>
          {Array.from(new Set([...selection.values, ...options]))
            .filter(
              (v) =>
                fetchValues || v.toLowerCase().includes(search.toLowerCase()),
            )
            .map((v) => (
              <label key={v}>
                <input
                  type={multiple ? "checkbox" : "radio"}
                  name={name}
                  value={v}
                  checked={!selection.all && selection.values.includes(v)}
                  onChange={(e) =>
                    setSelection({
                      all: false,
                      values: multiple
                        ? e.target.checked
                          ? [...selection.values.filter((p) => p !== v), v]
                          : selection.values.filter((p) => p !== v)
                        : [v],
                    })
                  }
                />
                {valueLabels?.[v] ?? v}
              </label>
            ))}
          {error && (
            <>
              <p role="alert">
                {text(
                  "候选查询失败，当前选值保持不变。",
                  "Candidate lookup failed. Your selection is unchanged.",
                )}
              </p>
              <Button onClick={() => setRetry(retry + 1)}>
                {text("重试", "Retry")}
              </Button>
            </>
          )}
          {cursor && (
            <Button
              disabled={loading}
              onClick={() => void load(search, cursor)}
            >
              {text("更多候选", "More candidates")}
            </Button>
          )}
          {loading && <p role="status">{text("正在查询…", "Loading…")}</p>}
        </div>
      </Dialog>
    </>
  );
}
