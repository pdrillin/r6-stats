import { useEffect, useRef, useState, type MouseEvent, type ReactNode } from "react";
import { createPortal } from "react-dom";

export interface Column<T> {
  label: string;
  /** How the column is computed, behind an ⓘ button next to the label. */
  title?: string;
  /** Sort key. Null values sort last. Columns without one are not sortable. */
  sort?: (row: T) => number | string | null;
  render: (row: T) => ReactNode;
  /** Text columns are left-aligned, the rest right-aligned. */
  text?: boolean;
  /** A secondary column, hidden until the reader asks for every column. */
  extra?: boolean;
  /** Left out of the phone cards, e.g. when the cards are grouped by it. */
  noCard?: boolean;
}

interface Props<T> {
  rows: T[];
  columns: Column<T>[];
  rowKey: (row: T) => string;
  /** Index of the column sorted at first, descending unless it is text. */
  initialSort?: number;
  /** Rows are sorted inside each group, groups keep their order. */
  group?: (row: T) => string;
  /** Remembers, in this browser, whether every column is shown. */
  id?: string;
}

/** Below this width rows are shown as cards: tables do not fit a phone. */
const NARROW = "(max-width: 700px)";

function useNarrow(): boolean {
  const [narrow, setNarrow] = useState(() => window.matchMedia(NARROW).matches);
  useEffect(() => {
    const mq = window.matchMedia(NARROW);
    const on = () => setNarrow(mq.matches);
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, []);
  return narrow;
}

function useShowAll(id?: string): [boolean, (v: boolean) => void] {
  const key = id && "r6stats.columns." + id;
  const [all, setAll] = useState(() => {
    try {
      return !!key && localStorage.getItem(key) === "all";
    } catch {
      return false;
    }
  });
  const set = (v: boolean) => {
    setAll(v);
    try {
      if (key) {
        if (v) localStorage.setItem(key, "all");
        else localStorage.removeItem(key);
      }
    } catch {
      // A preference only: without storage it lasts until the page closes.
    }
  };
  return [all, set];
}

export function Table<T>({ rows, columns, rowKey, initialSort, group, id }: Props<T>) {
  const [sortBy, setSortBy] = useState<number | undefined>(initialSort);
  const [desc, setDesc] = useState(initialSort !== undefined && !columns[initialSort]?.text);
  const [showAll, setShowAll] = useShowAll(id);
  const narrow = useNarrow();

  const extras = columns.filter((c) => c.extra).length;
  // Indexes into columns, so sorting keeps pointing at the same column.
  const visible = columns.map((c, i) => ({ c, i })).filter(({ c }) => showAll || !c.extra);

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

  const sort = (i: number, direction?: boolean) => {
    if (!columns[i]?.sort) return;
    if (direction !== undefined) {
      setSortBy(i);
      setDesc(direction);
    } else if (i === sortBy) setDesc(!desc);
    else {
      setSortBy(i);
      setDesc(!columns[i]?.text);
    }
  };

  const toggle = extras > 0 && (
    <button className="link-button columns-toggle" onClick={() => setShowAll(!showAll)} aria-pressed={showAll}>
      {showAll ? "Moins de colonnes" : `Plus de colonnes (${extras})`}
    </button>
  );

  if (narrow) {
    return (
      <Cards
        rows={shown}
        visible={visible}
        rowKey={rowKey}
        group={group}
        sortBy={sortBy}
        desc={desc}
        onSort={sort}
        toggle={toggle}
      />
    );
  }

  return (
    <div className="table-block">
      {toggle && <div className="table-tools">{toggle}</div>}
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              {visible.map(({ c, i }) => (
                <th
                  key={c.label}
                  className={[c.text ? "text" : "", i === sortBy ? "sorted" : ""].join(" ")}
                  aria-sort={i === sortBy ? (desc ? "descending" : "ascending") : undefined}
                >
                  <span className="th-inner">
                    {c.sort ? (
                      <button className="th-sort" onClick={() => sort(i)}>
                        {c.label}
                        {i === sortBy && <span className="arrow">{desc ? "▾" : "▴"}</span>}
                      </button>
                    ) : (
                      c.label
                    )}
                    {c.title && <Help label={c.label} text={c.title} />}
                  </span>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {shown.map((r, k) => {
              const newGroup = group && k > 0 && group(r) !== group(shown[k - 1]!);
              return (
                <tr key={rowKey(r)} className={newGroup ? "group-start" : undefined}>
                  {visible.map(({ c }) => (
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
    </div>
  );
}

/** Rows as cards on a phone: the first column titles each card. */
function Cards<T>({
  rows,
  visible,
  rowKey,
  group,
  sortBy,
  desc,
  onSort,
  toggle,
}: {
  rows: T[];
  visible: { c: Column<T>; i: number }[];
  rowKey: (row: T) => string;
  group?: (row: T) => string;
  sortBy?: number;
  desc: boolean;
  onSort: (i: number, desc?: boolean) => void;
  toggle: ReactNode;
}) {
  const [head, ...rest] = visible.filter(({ c }) => !c.noCard);
  const sortable = visible.filter(({ c }) => c.sort);
  return (
    <div className="table-block">
      <div className="table-tools">
        {/* Sorting a handful of cards is not worth a control. */}
        {sortable.length > 0 && rows.length > 3 && (
          <label className="sort-select">
            <span className="muted">Trier par</span>
            <select
              value={sortBy === undefined ? "" : `${sortBy}:${desc ? "d" : "a"}`}
              onChange={(e) => {
                const [i, d] = e.target.value.split(":");
                onSort(Number(i), d === "d");
              }}
            >
              {sortable.flatMap(({ c, i }) => [
                <option key={`${i}d`} value={`${i}:d`}>
                  {c.label} {c.text ? "(Z → A)" : "(du plus haut)"}
                </option>,
                <option key={`${i}a`} value={`${i}:a`}>
                  {c.label} {c.text ? "(A → Z)" : "(du plus bas)"}
                </option>,
              ])}
            </select>
          </label>
        )}
        {toggle}
      </div>
      <ul className="row-cards">
        {rows.map((r, k) => {
          const g = group?.(r);
          const newGroup = group && (k === 0 || g !== group(rows[k - 1]!));
          return (
            <li key={rowKey(r)} className="row-card-item">
              {newGroup && <div className="row-group">{g}</div>}
              <div className="row-card">
                {head && <div className="row-card-title">{head.c.render(r)}</div>}
                <dl>
                  {rest.map(({ c }) => (
                    <div key={c.label}>
                      <dt>
                        {c.label}
                        {c.title && <Help label={c.label} text={c.title} />}
                      </dt>
                      <dd>{c.render(r)}</dd>
                    </div>
                  ))}
                </dl>
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

/**
 * An ⓘ button explaining a column. It opens on click or tap, so it works
 * without a mouse, and closes with Escape or a click elsewhere. The bubble is
 * drawn over the page: a table's scroll box would clip it.
 */
export function Help({ label, text }: { label: string; text: string }) {
  const [at, setAt] = useState<{ x: number; y: number } | null>(null);
  const button = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!at) return;
    const close = (e: Event) => {
      if (e instanceof KeyboardEvent && e.key !== "Escape") return;
      // A press on the button itself toggles it: its own click handles it.
      if (e.type === "pointerdown" && e.target === button.current) return;
      setAt(null);
    };
    window.addEventListener("keydown", close);
    window.addEventListener("pointerdown", close);
    window.addEventListener("scroll", close, true);
    window.addEventListener("resize", close);
    return () => {
      window.removeEventListener("keydown", close);
      window.removeEventListener("pointerdown", close);
      window.removeEventListener("scroll", close, true);
      window.removeEventListener("resize", close);
    };
  }, [at]);
  const toggle = (e: MouseEvent) => {
    e.stopPropagation();
    if (at) {
      setAt(null);
      return;
    }
    const r = button.current!.getBoundingClientRect();
    setAt({ x: r.left + r.width / 2, y: r.bottom + 6 });
  };
  const width = Math.min(280, window.innerWidth - 24);
  const left = at ? Math.max(12, Math.min(at.x - width / 2, window.innerWidth - width - 12)) : 0;
  return (
    <>
      <button
        ref={button}
        className="help-button"
        aria-label={`Qu'est-ce que « ${label} » ?`}
        aria-expanded={!!at}
        onClick={toggle}
      >
        ⓘ
      </button>
      {at &&
        createPortal(
          <div className="help-bubble" role="tooltip" style={{ left, top: at.y, width }}>
            <b>{label}</b> : {text}
          </div>,
          document.body,
        )}
    </>
  );
}
