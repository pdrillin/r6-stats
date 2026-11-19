import type { ReactNode } from "react";
import { Link, useLocation, useSearchParams, type LinkProps } from "react-router-dom";
import { useApi, type Overview } from "../api";

/** The map the statistics are filtered on, from the URL (?map=), "" for all. */
export function useMapFilter(): [string, (map: string) => void] {
  const [params, setParams] = useSearchParams();
  const map = params.get("map") ?? "";
  const set = (m: string) =>
    setParams(
      (p) => {
        if (m) p.set("map", m);
        else p.delete("map");
        return p;
      },
      { replace: true },
    );
  return [map, set];
}

/** The overview for the current map filter. */
export function useOverview() {
  const [map] = useMapFilter();
  return useApi<Overview>(map ? "overview?map=" + encodeURIComponent(map) : "overview");
}

/** A link that keeps the map filter. */
export function FLink({ to, ...rest }: Omit<LinkProps, "to"> & { to: string }) {
  const { search } = useLocation();
  const map = new URLSearchParams(search).get("map");
  const target = map ? `${to}?map=${encodeURIComponent(map)}` : to;
  return <Link to={target} {...rest} />;
}

/** The filter row, above everything it scopes. */
export function MapFilter({ maps, children }: { maps: string[]; children?: ReactNode }) {
  const [map, setMap] = useMapFilter();
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
      {map && (
        <button className="link-button" onClick={() => setMap("")}>
          Retirer le filtre
        </button>
      )}
      {children}
    </div>
  );
}
