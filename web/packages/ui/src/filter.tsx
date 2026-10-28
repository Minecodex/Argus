import { RefreshCw, Search } from "lucide-react";
import { forwardRef, type InputHTMLAttributes, type ReactNode } from "react";
import { Button } from "./button";
import { cx } from "./lib";
import { useUiText } from "./locale";
import { Select } from "./select";
import { Field, Input } from "./form";

export type SearchInputProps = InputHTMLAttributes<HTMLInputElement>;

export const SearchInput = forwardRef<HTMLInputElement, SearchInputProps>(
  ({ className, ...props }, ref) => (
    <div className={cx("argus-search-input", className)}>
      <Search aria-hidden size={15} />
      <input ref={ref} type="search" {...props} />
    </div>
  ),
);
SearchInput.displayName = "SearchInput";

export type FilterBarFilter = {
  key: string;
  value: string;
  options: Array<{ value: string; label: string }>;
  /** Placeholder option shown with an empty value, e.g. "All statuses". */
  allLabel?: string;
  ariaLabel?: string;
  onChange: (value: string) => void;
};

export type FilterBarDateRange = {
  from: string;
  to: string;
  fromLabel?: string;
  toLabel?: string;
  onFromChange: (value: string) => void;
  onToChange: (value: string) => void;
};

export function FilterBar({
  search,
  filters = [],
  dateRange,
  inlineActions,
  actions,
  onRefresh,
  refreshing,
  refreshLabel,
  className,
}: {
  search?: {
    value: string;
    onChange: (value: string) => void;
    placeholder?: string;
  };
  filters?: FilterBarFilter[];
  dateRange?: FilterBarDateRange;
  /** Controls adjacent to the filters; actions and refresh remain at the end. */
  inlineActions?: ReactNode;
  actions?: ReactNode;
  onRefresh?: () => void;
  refreshing?: boolean;
  refreshLabel?: string;
  className?: string;
}) {
  const text = useUiText();
  return (
    <div
      className={cx(
        "argus-filter-bar",
        Boolean(inlineActions) && "argus-filter-bar--grouped",
        className,
      )}
    >
      {search && (
        <SearchInput
          aria-label={search.placeholder ?? text("搜索", "Search")}
          className="argus-filter-bar__search"
          onChange={(event) => search.onChange(event.target.value)}
          placeholder={search.placeholder ?? text("搜索…", "Search…")}
          value={search.value}
        />
      )}
      {filters.map((filter) => (
        <div className="argus-filter-bar__select" key={filter.key}>
          <Select
            sizing="content"
            ariaLabel={filter.ariaLabel ?? filter.allLabel ?? filter.key}
            onValueChange={filter.onChange}
            options={[
              ...(filter.allLabel !== undefined
                ? [{ value: "", label: filter.allLabel }]
                : []),
              ...filter.options,
            ]}
            value={filter.value}
          />
        </div>
      ))}
      {dateRange && (
        <div className="argus-filter-bar__dates">
          <Field
            className="argus-filter-bar__date"
            label={dateRange.fromLabel ?? text("开始日期", "Start date")}
            requirement="optional"
          >
            <Input
              type="date"
              value={dateRange.from}
              onChange={(event) => dateRange.onFromChange(event.target.value)}
            />
          </Field>
          <Field
            className="argus-filter-bar__date"
            label={dateRange.toLabel ?? text("结束日期", "End date")}
            requirement="optional"
          >
            <Input
              type="date"
              value={dateRange.to}
              onChange={(event) => dateRange.onToChange(event.target.value)}
            />
          </Field>
        </div>
      )}
      {inlineActions && (
        <div className="argus-filter-bar__inline-actions">{inlineActions}</div>
      )}
      {actions && <div className="argus-filter-bar__actions">{actions}</div>}
      {onRefresh && (
        <Button
          aria-label={refreshLabel ?? text("刷新", "Refresh")}
          className="argus-filter-bar__refresh"
          onPress={onRefresh}
          isIconOnly
          variant="ghost"
        >
          <RefreshCw
            aria-hidden
            className={refreshing ? "argus-spin" : undefined}
            size={15}
          />
        </Button>
      )}
    </div>
  );
}
