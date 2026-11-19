import type { ReactNode } from "react";
import { SMALL_SAMPLE, percent } from "../format";

export interface RateRow {
  key: string;
  label: ReactNode;
  n: number;
  of: number;
  /** Text after the sample, e.g. "posé 3 fois". */
  note?: ReactNode;
}

/**
 * One bar per row for a rate, on a shared 0-100 % scale with a 50 % mark.
 * The value and its sample are printed beside each bar.
 */
export function RateBars({ rows, label }: { rows: RateRow[]; label: string }) {
  if (rows.length === 0) return <p className="muted">Aucune donnée.</p>;
  return (
    <div className="rate-bars" role="table" aria-label={label}>
      {rows.map((r) => {
        const small = r.of < SMALL_SAMPLE;
        const pct = r.of > 0 ? (100 * r.n) / r.of : 0;
        return (
          <div key={r.key} className="rate-row" role="row" title={`${r.n} sur ${r.of}`}>
            <div className="rate-label" role="rowheader">
              {r.label}
            </div>
            <div className="rate-track" role="cell">
              <span className="rate-fill" style={{ width: `${pct}%` }} />
              <span className="rate-half" aria-hidden="true" />
            </div>
            <div className={small ? "rate-value small" : "rate-value"} role="cell">
              <b>{percent(r.n, r.of)}</b> <span className="sample">{r.n}/{r.of}</span>
              {r.note && <span className="sample"> · {r.note}</span>}
            </div>
          </div>
        );
      })}
    </div>
  );
}
