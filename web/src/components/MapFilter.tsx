import type { ReactNode } from "react";
import { Link, useLocation, useSearchParams, type LinkProps } from "react-router-dom";
import { kindLabels, useApi, type MatchKind, type MatchSummary, type Overview } from "../api";

// The filters live in the URL (?map=…&kind=…), so they follow the reader from
// page to page and a filtered page can be shared.
const FILTERS = ["map", "kind"] as const;
type FilterName = (typeof FILTERS)[number];

function useFilter(name: FilterName): [string, (value: string) => void] {
  const [params, setParams] = useSearchParams();
  const value = params.get(name) ?? "";
  const set = (v: string) =>
    setParams(
      (p) => {
        if (v) p.set(name, v);
        else p.delete(name);
        return p;
      },
      { replace: true },
    );
  return [value, set];
}

/** The map the statistics are filtered on, "" for all. */
export function useMapFilter() {
  return useFilter("map");
}

/** The kind of match the statistics are filtered on, "" for all. */
export function useKindFilter(): [MatchKind | "", (kind: string) => void] {
  const [kind, set] = useFilter("kind");
  return [kind in kindLabels ? (kind as MatchKind) : "", set];
}

/** Whether a match passes the current filters. */
export function useMatchFilter(): (m: MatchSummary) => boolean {
  const [map] = useMapFilter();
  const [kind] = useKindFilter();
  return (m) => (!map || m.map === map) && (!kind || m.kind === kind);
}

/** The filters in words, to complete a sentence: " sur Bank, en officiel". */
export function scope(f: { map: string; kind: MatchKind | "" }): string {
  const parts = [];
  if (f.map) parts.push(`sur ${f.map}`);
  if (f.kind) parts.push(f.kind === "official" ? "en match officiel" : "en pracc");
  return parts.length ? " " + parts.join(", ") : "";
}

/** The query string holding only the filters of search, with its "?". */
export function filterSearch(search: string): string {
  const from = new URLSearchParams(search);
  const to = new URLSearchParams();
  for (const f of FILTERS) {
    const v = from.get(f);
    if (v) to.set(f, v);
  }
  const q = to.toString();
  return q ? "?" + q : "";
}

/** The overview for the current filters. */
export function useOverview() {
  return useApi<Overview>("overview" + filterSearch(useLocation().search));
}

/** A link that keeps the filters. */
export function FLink({ to, ...rest }: Omit<LinkProps, "to"> & { to: string }) {
  const { search } = useLocation();
  return <Link to={to + filterSearch(search)} {...rest} />;
}

/** The filter row, above everything it scopes. */
export function MapFilter({ maps, children }: { maps: string[]; children?: ReactNode }) {
  const [map, setMap] = useMapFilter();
  const [kind, setKind] = useKindFilter();
  const [, setParams] = useSearchParams();
  const clearAll = () =>
    setParams(
      (p) => {
        for (const f of FILTERS) p.delete(f);
        return p;
      },
      { replace: true },
    );
  return (
    <div className="filters">
      <label>
        <span className="muted">Map</span>
        <select value={map} onChange={(e) => setMap(e.target.value)}>
          <option value="">Toutes les maps</option>
          {maps.map((m) => (
            <option key={m} value={m}>
              {m}
            </option>
          ))}
        </select>
      </label>
      <label>
        <span className="muted">Type</span>
        <select value={kind} onChange={(e) => setKind(e.target.value)}>
          <option value="">Tous les matchs</option>
          {(Object.keys(kindLabels) as MatchKind[]).map((k) => (
            <option key={k} value={k}>
              {kindLabels[k]}
            </option>
          ))}
        </select>
      </label>
      {(map || kind) && (
        <button className="link-button" onClick={clearAll}>
          Retirer les filtres
        </button>
      )}
      {children}
    </div>
  );
}
