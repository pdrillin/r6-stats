import { useState, type ReactNode } from "react";

export interface Column<T> {
  label: string;
  /** Tooltip on the header: how the column is computed. */
  title?: string;
  /** Sort key. Null values sort last. Columns without one are not sortable. */
  sort?: (row: T) => number | string | null;
  render: (row: T) => ReactNode;
  /** Text columns are left-aligned, the rest right-aligned. */
  text?: boolean;
}

interface Props<T> {
  rows: T[];
  columns: Column<T>[];
  rowKey: (row: T) => string;
  /** Index of the column sorted at first, descending unless it is text. */
  initialSort?: number;
  /** Rows are sorted inside each group, groups keep their order. */
  group?: (row: T) => string;
}

export function Table<T>({ rows, columns, rowKey, initialSort, group }: Props<T>) {
  const [sortBy, setSortBy] = useState<number | undefined>(initialSort);
  const [desc, setDesc] = useState(initialSort !== undefined && !columns[initialSort]?.text);

  let shown = rows;
  const col = sortBy === undefined ? undefined : columns[sortBy];
  if (col?.sort) {
    const key = col.sort;
    const order = new Map<string, number>();
    if (group) for (const r of rows) if (!order.has(group(r))) order.set(group(r), order.size);
    shown = [...rows].sort((a, b) => {
      if (group) {
        const g = order.get(group(a))! - order.get(group(b))!;
        if (g !== 0) return g;
      }
      const x = key(a);
      const y = key(b);
      if (x === y) return 0;
      if (x === null) return 1;
      if (y === null) return -1;
      const c = x < y ? -1 : 1;
      return desc ? -c : c;
    });
  }

  const click = (i: number) => {
    if (!columns[i]?.sort) return;
    if (i === sortBy) setDesc(!desc);
    else {
      setSortBy(i);
      setDesc(!columns[i]?.text);
    }
  };

  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            {columns.map((c, i) => (
              <th
                key={c.label}
                title={c.title}
                className={[c.text ? "text" : "", c.sort ? "sortable" : "", i === sortBy ? "sorted" : ""].join(" ")}
                onClick={() => click(i)}
                aria-sort={i === sortBy ? (desc ? "descending" : "ascending") : undefined}
              >
                {c.label}
                {i === sortBy && <span className="arrow">{desc ? "▾" : "▴"}</span>}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {shown.map((r, i) => {
            const newGroup = group && i > 0 && group(r) !== group(shown[i - 1]!);
            return (
              <tr key={rowKey(r)} className={newGroup ? "group-start" : undefined}>
                {columns.map((c) => (
                  <td key={c.label} className={c.text ? "text" : undefined}>
                    {c.render(r)}
                  </td>
                ))}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
