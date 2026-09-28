export type DashboardRectangle = {
  x: number;
  y: number;
  w: number;
  h: number;
  min_w: number;
  min_h: number;
};
export type DashboardLayoutItem = { id: string; layout: DashboardRectangle };
const overlaps = (a: DashboardRectangle, b: DashboardRectangle) =>
  a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h;
export function arrangeDashboard<T extends DashboardLayoutItem>(
  items: T[],
  id: string,
  proposed: DashboardRectangle,
): T[] {
  const moving = items.find((i) => i.id === id);
  if (!moving) return items;
  const layout = {
    ...proposed,
    x: Math.max(0, Math.min(12 - proposed.w, Math.round(proposed.x))),
    y: Math.max(0, Math.round(proposed.y)),
    w: Math.max(proposed.min_w, Math.min(12, Math.round(proposed.w))),
    h: Math.max(proposed.min_h, Math.min(1000, Math.round(proposed.h))),
  };
  layout.x = Math.min(layout.x, 12 - layout.w);
  const placed: DashboardRectangle[] = [layout];
  const result = new Map<string, DashboardRectangle>([[id, layout]]);
  const ordered = items
    .filter((i) => i.id !== id)
    .sort(
      (a, b) =>
        a.layout.y - b.layout.y ||
        a.layout.x - b.layout.x ||
        a.id.localeCompare(b.id),
    );
  for (const item of ordered) {
    const r = { ...item.layout };
    for (;;) {
      const collision = placed.filter((p) => overlaps(r, p));
      if (!collision.length) break;
      r.y = Math.max(...collision.map((p) => p.y + p.h));
    }
    placed.push(r);
    result.set(item.id, r);
  }
  return items.map((item) => ({ ...item, layout: result.get(item.id)! }));
}
