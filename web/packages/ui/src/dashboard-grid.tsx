import {
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type PointerEvent,
  type ReactNode,
} from "react";
import { GripHorizontal, MoveDiagonal2 } from "lucide-react";
import { DashboardHandle } from "./dashboard-handle";
import { useUiText } from "./locale";
import { arrangeDashboard, type DashboardLayoutItem } from "./dashboard-layout";

export function DashboardGrid<
  T extends DashboardLayoutItem & { title: string },
>({
  items,
  rowHeight,
  editable,
  onChange,
  children,
}: {
  items: T[];
  rowHeight: number;
  editable?: boolean;
  onChange?: (items: T[]) => void;
  children: (item: T) => ReactNode;
}) {
  const root = useRef<HTMLDivElement>(null),
    gesture = useRef<
      | {
          id: string;
          kind: "move" | "resize";
          x: number;
          y: number;
          initial: T[];
          width: number;
          pointerId: number;
        }
      | undefined
    >(undefined);
  const [preview, setPreview] = useState<T[] | null>(null);
  const latest = useRef<T[] | null>(null);
  const text = useUiText();
  useEffect(() => {
    if (!gesture.current) {
      setPreview(null);
      latest.current = null;
    }
  }, [items]);
  const start = (
    event: PointerEvent<HTMLButtonElement>,
    item: T,
    kind: "move" | "resize",
  ) => {
    if (event.button !== 0 || gesture.current) return;
    event.preventDefault();
    event.currentTarget.focus({ preventScroll: true });
    event.currentTarget.setPointerCapture(event.pointerId);
    latest.current = null;
    gesture.current = {
      id: item.id,
      kind,
      x: event.clientX,
      y: event.clientY,
      initial: items,
      width: root.current?.clientWidth ?? 1200,
      pointerId: event.pointerId,
    };
  };
  const move = (event: PointerEvent<HTMLButtonElement>) => {
    const g = gesture.current;
    if (!g || g.pointerId !== event.pointerId) return;
    const item = g.initial.find((i) => i.id === g.id)!;
    const dx = Math.round((event.clientX - g.x) / (g.width / 12)),
      dy = Math.round((event.clientY - g.y) / rowHeight);
    const r = item.layout;
    const next =
      g.kind === "move"
        ? {
            ...r,
            x: Math.max(0, Math.min(12 - r.w, r.x + dx)),
            y: Math.max(0, r.y + dy),
          }
        : {
            ...r,
            w: Math.max(r.min_w, Math.min(12 - r.x, r.w + dx)),
            h: Math.max(r.min_h, r.h + dy),
          };
    latest.current = arrangeDashboard(g.initial, item.id, next);
    setPreview(latest.current);
  };
  const finish = (event: PointerEvent<HTMLButtonElement>) => {
    const g = gesture.current;
    if (!g || g.pointerId !== event.pointerId) return;
    if (
      latest.current &&
      latest.current.some(
        (item, index) =>
          JSON.stringify(item.layout) !==
          JSON.stringify(g.initial[index]!.layout),
      )
    )
      onChange?.(latest.current);
    gesture.current = undefined;
    latest.current = null;
    setPreview(null);
  };
  const cancel = () => {
    gesture.current = undefined;
    latest.current = null;
    setPreview(null);
  };
  const keyboard = (
    event: KeyboardEvent<HTMLButtonElement>,
    item: T,
    resize: boolean,
  ) => {
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      cancel();
      return;
    }
    const deltas: Record<string, [number, number]> = {
      ArrowLeft: [-1, 0],
      ArrowRight: [1, 0],
      ArrowUp: [0, -1],
      ArrowDown: [0, 1],
    };
    const delta = deltas[event.key];
    if (!delta) return;
    event.preventDefault();
    cancel();
    const [dx, dy] = delta,
      r = item.layout;
    onChange?.(
      arrangeDashboard(
        items,
        item.id,
        resize || event.shiftKey
          ? {
              ...r,
              w: Math.max(r.min_w, Math.min(12 - r.x, r.w + dx)),
              h: Math.max(r.min_h, r.h + dy),
            }
          : {
              ...r,
              x: Math.max(0, Math.min(12 - r.w, r.x + dx)),
              y: Math.max(0, r.y + dy),
            },
      ),
    );
  };
  return (
    <div
      ref={root}
      className="argus-dashboard-grid"
      style={{ "--argus-dashboard-row": `${rowHeight}px` } as CSSProperties}
    >
      {(preview ?? items).map((item) => (
        <section
          key={item.id}
          data-panel-id={item.id}
          className="argus-dashboard-tile"
          style={{
            gridColumn: `${item.layout.x + 1} / span ${item.layout.w}`,
            gridRow: `${item.layout.y + 1} / span ${item.layout.h}`,
          }}
        >
          {editable && (
            <DashboardHandle
              className="argus-dashboard-move"
              aria-label={`${text("移动统计图", "Move panel")} ${item.title}`}
              title={text(
                "方向键移动，Shift + 方向键缩放",
                "Arrow keys move; Shift + arrow keys resize",
              )}
              onPointerDown={(e) => start(e, item, "move")}
              onPointerMove={move}
              onPointerUp={finish}
              onPointerCancel={cancel}
              onLostPointerCapture={cancel}
              onKeyDown={(e) => keyboard(e, item, false)}
            >
              <GripHorizontal size={16} />
            </DashboardHandle>
          )}
          {children(item)}
          {editable && (
            <DashboardHandle
              className="argus-dashboard-resize"
              aria-label={`${text("调整统计图尺寸", "Resize panel")} ${item.title}`}
              onPointerDown={(e) => start(e, item, "resize")}
              onPointerMove={move}
              onPointerUp={finish}
              onPointerCancel={cancel}
              onLostPointerCapture={cancel}
              onKeyDown={(e) => keyboard(e, item, true)}
            >
              <MoveDiagonal2 size={14} />
            </DashboardHandle>
          )}
        </section>
      ))}
    </div>
  );
}
