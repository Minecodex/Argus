import type { ReactNode } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "./button";
import { useUiText } from "./locale";

/** Cursor lists omit totalPages: only reached pages and the next cursor are shown. */
export function Pagination({
  page,
  totalPages,
  maxReachablePage = totalPages ?? page + 1,
  hasPrevious = page > 1,
  hasNext = totalPages !== undefined && page < totalPages,
  isDisabled = false,
  onChange,
  onPrevious,
  onNext,
  onFirst,
  summary,
  ariaLabel,
}: {
  page: number;
  totalPages?: number;
  maxReachablePage?: number;
  hasPrevious?: boolean;
  hasNext?: boolean;
  isDisabled?: boolean;
  onChange?: (page: number) => void;
  onPrevious?: () => void;
  onNext?: () => void;
  onFirst?: () => void;
  summary?: ReactNode;
  ariaLabel?: string;
}) {
  const text = useUiText();
  const total = totalPages === undefined ? undefined : Math.max(1, totalPages);
  const current = Math.max(
    1,
    total === undefined ? page : Math.min(page, total),
  );
  const end = total ?? current + (hasNext ? 1 : 0);
  const numbers = new Set([1, end]);
  for (let n = Math.max(1, current - 2); n <= Math.min(end, current + 2); n++)
    numbers.add(n);
  const pages: Array<number | string> = [];
  [...numbers]
    .sort((a, b) => a - b)
    .forEach((n, i, all) => {
      if (i && n - all[i - 1]! > 1) pages.push(`gap-${n}`);
      pages.push(n);
    });
  const reachable = (n: number) =>
    n === current ||
    (n < current &&
      hasPrevious &&
      (!!onChange ||
        (n === 1 && !!onFirst) ||
        (n === current - 1 && !!onPrevious))) ||
    (n > current &&
      n <= maxReachablePage &&
      hasNext &&
      (!!onChange || (n === current + 1 && !!onNext)));
  const change = (n: number) => {
    if (isDisabled || n === current || !reachable(n)) return;
    if (onChange) onChange(n);
    else if (n === current - 1 && onPrevious) onPrevious();
    else if (n === current + 1 && onNext) onNext();
    else if (n === 1) onFirst?.();
  };
  return (
    <nav
      aria-label={ariaLabel ?? text("分页", "Pagination")}
      className="argus-pagination"
      aria-busy={isDisabled || undefined}
    >
      <span className="argus-pagination__summary">
        {summary ??
          (total === undefined
            ? text(`第 ${current} 页`, `Page ${current}`)
            : text(
                `第 ${current} / ${total} 页`,
                `Page ${current} of ${total}`,
              ))}
      </span>
      <div className="argus-pagination__pages">
        <Button
          aria-label={text("上一页", "Previous page")}
          isDisabled={isDisabled || !hasPrevious}
          onPress={() => change(current - 1)}
          isIconOnly
          size="sm"
          variant="ghost"
        >
          <ChevronLeft size={16} aria-hidden />
        </Button>
        {pages.map((n) =>
          typeof n === "string" ? (
            <span key={n} className="argus-pagination__ellipsis" aria-hidden>
              …
            </span>
          ) : (
            <Button
              key={n}
              aria-label={text(`第 ${n} 页`, `Page ${n}`)}
              aria-current={n === current ? "page" : undefined}
              isDisabled={isDisabled || !reachable(n)}
              onPress={() => change(n)}
              size="sm"
              variant={n === current ? "primary" : "ghost"}
              className="argus-pagination__page"
            >
              {n}
            </Button>
          ),
        )}
        <Button
          aria-label={text("下一页", "Next page")}
          isDisabled={isDisabled || !hasNext}
          onPress={() => change(current + 1)}
          isIconOnly
          size="sm"
          variant="ghost"
        >
          <ChevronRight size={16} aria-hidden />
        </Button>
      </div>
    </nav>
  );
}
