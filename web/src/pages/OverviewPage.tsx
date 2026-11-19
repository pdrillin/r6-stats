import type { Overview } from "../api";
import { Loading } from "../components/Loading";
import { MapFilter, useOverview } from "../components/MapFilter";
import { PlayerTable } from "../components/PlayerTable";

export function OverviewPage() {
  const state = useOverview();
  return (
    <Loading state={state}>
      {(o) => (
        <>
          <h1>Joueurs</h1>
          <MapFilter maps={o.maps} />
          <Summary o={o} />
          <PlayerTable players={o.stats.players} />
        </>
      )}
    </Loading>
  );
}

export function Summary({ o }: { o: Overview }) {
  return (
    <p className="muted summary">
      {o.map && <>Sur {o.map} : </>}
      {o.matches} match{o.matches > 1 ? "s" : ""}, {o.stats.rounds} rounds
      {o.stats.skippedRounds > 0 && ` (${o.stats.skippedRounds} écartés : vainqueur inconnu)`}. Fenêtre de trade :{" "}
      {o.tradeWindowMs / 1000} s. Les pourcentages en gris reposent sur moins de 10 cas.
    </p>
  );
}
