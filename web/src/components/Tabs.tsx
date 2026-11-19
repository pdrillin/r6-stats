import { useRef, type KeyboardEvent, type ReactNode } from "react";
import { useSearchParams } from "react-router-dom";

export interface Tab {
  id: string;
  label: string;
}

/**
 * The tab shown, kept in the URL (?vue=…) so a tab can be linked to and the
 * browser's back button returns to the previous one. The first tab is the
 * default and leaves the URL clean.
 */
export function useTab(tabs: Tab[]): [string, (id: string) => void] {
  const [params, setParams] = useSearchParams();
  const asked = params.get("vue");
  const current = tabs.some((t) => t.id === asked) ? asked! : tabs[0]!.id;
  const set = (id: string) =>
    setParams((p) => {
      if (id === tabs[0]!.id) p.delete("vue");
      else p.set("vue", id);
      return p;
    });
  return [current, set];
}

/** Tab buttons, keyboard included: arrows move between tabs. */
export function TabList({
  tabs,
  current,
  onChange,
  label,
}: {
  tabs: Tab[];
  current: string;
  onChange: (id: string) => void;
  label: string;
}) {
  const refs = useRef<(HTMLButtonElement | null)[]>([]);
  const onKey = (e: KeyboardEvent, i: number) => {
    const step = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
    const to = e.key === "Home" ? 0 : e.key === "End" ? tabs.length - 1 : (i + step + tabs.length) % tabs.length;
    if (step === 0 && e.key !== "Home" && e.key !== "End") return;
    e.preventDefault();
    onChange(tabs[to]!.id);
    refs.current[to]?.focus();
  };
  return (
    <div className="tablist" role="tablist" aria-label={label}>
      {tabs.map((t, i) => (
        <button
          key={t.id}
          ref={(el) => {
            refs.current[i] = el;
          }}
          role="tab"
          id={`tab-${t.id}`}
          aria-selected={t.id === current}
          aria-controls={`panel-${t.id}`}
          tabIndex={t.id === current ? 0 : -1}
          className={t.id === current ? "active" : undefined}
          onClick={() => onChange(t.id)}
          onKeyDown={(e) => onKey(e, i)}
        >
          {t.label}
        </button>
      ))}
    </div>
  );
}

export function TabPanel({ id, children }: { id: string; children: ReactNode }) {
  return (
    <div role="tabpanel" id={`panel-${id}`} aria-labelledby={`tab-${id}`} className="tabpanel">
      {children}
    </div>
  );
}
