import type { ReactNode } from "react";

interface Props<T> {
  state: { data?: T; error?: string };
  children: (data: T) => ReactNode;
}

/** Renders children once the API answered, a message until then. */
export function Loading<T>({ state, children }: Props<T>) {
  if (state.error) return <p className="error">Impossible de charger les données : {state.error}</p>;
  if (state.data === undefined) return <p className="muted">Chargement…</p>;
  return <>{children(state.data)}</>;
}
