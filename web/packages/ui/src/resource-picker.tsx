import type { ReactNode } from "react";
import { Button } from "./button";
import { Input } from "./form";
import { Dialog } from "./primitives";
import { DataTable, Pagination, type Column } from "./table";
import { useUiText } from "./locale";

export function ResourcePickerDialog<T extends object>({
  open,
  onOpenChange,
  title,
  description,
  data,
  columns,
  getRowKey,
  getRowLabel,
  search,
  onSearch,
  loading,
  error,
  page = 1,
  hasPrevious,
  hasNext,
  onPrevious,
  onNext,
  selectedKeys = [],
  onSelect,
  onToggle,
  onConfirm,
  isDisabled,
  filters,
  confirmDisabled = false,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  data: T[];
  columns: Column<T>[];
  getRowKey: (item: T) => string;
  getRowLabel: (item: T) => string;
  search: string;
  onSearch: (value: string) => void;
  loading?: boolean;
  error?: ReactNode;
  page?: number;
  hasPrevious?: boolean;
  hasNext?: boolean;
  onPrevious?: () => void;
  onNext?: () => void;
  selectedKeys?: string[];
  onSelect?: (item: T) => void;
  onToggle?: (item: T) => void;
  onConfirm?: () => void;
  isDisabled?: (item: T) => boolean;
  filters?: ReactNode;
  confirmDisabled?: boolean;
}) {
  const text = useUiText();
  const rows: Column<T>[] = [
    ...columns,
    {
      key: "picker_action",
      header: text("选择", "Selection"),
      align: "right",
      render: (item) => (
        <Button
          type="button"
          isDisabled={isDisabled?.(item)}
          size="sm"
          aria-label={`${text("选择", "Select")} ${getRowLabel(item)}`}
          aria-pressed={
            onToggle ? selectedKeys.includes(getRowKey(item)) : undefined
          }
          onPress={() => {
            if (onToggle) onToggle(item);
            else {
              onSelect?.(item);
              onOpenChange(false);
            }
          }}
        >
          {selectedKeys.includes(getRowKey(item))
            ? text("已选择", "Selected")
            : text("选择", "Select")}
        </Button>
      ),
    },
  ];
  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={title}
      description={description}
      size="lg"
      footer={
        <>
          <Button onPress={() => onOpenChange(false)}>
            {text("取消", "Cancel")}
          </Button>
          {onConfirm && (
            <Button
              variant="primary"
              isDisabled={confirmDisabled}
              onPress={() => {
                onConfirm();
                onOpenChange(false);
              }}
            >
              {text("确认选择", "Confirm selection")}
              {selectedKeys.length ? ` (${selectedKeys.length})` : ""}
            </Button>
          )}
        </>
      }
    >
      <div className="argus-resource-picker">
        {filters}
        <Input
          aria-label={text("搜索列表", "Search list")}
          placeholder={text("搜索名称或标识", "Search name or identifier")}
          value={search}
          onChange={(event) => onSearch(event.target.value)}
        />
        {error && <div role="alert">{error}</div>}
        {loading && <p role="status">{text("正在加载…", "Loading…")}</p>}
        <DataTable
          columns={rows}
          data={data}
          getRowKey={getRowKey}
          density="compact"
        />
        {!loading && !error && data.length === 0 && (
          <p role="status">{text("没有匹配的结果", "No matching results")}</p>
        )}
        <Pagination
          page={page}
          hasPrevious={hasPrevious}
          hasNext={hasNext}
          isDisabled={loading}
          onPrevious={onPrevious}
          onNext={onNext}
        />
      </div>
    </Dialog>
  );
}
