import { useApi, type Overview } from "../api";
import { Loading } from "../components/Loading";
import { PlayerTable } from "../components/PlayerTable";

export function OverviewPage() {
  const state = useApi<Overview>("overview");
  return (
    <Loading state={state}>
      {(o) => (
        <>
          <h1>Joueurs</h1>
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
      {o.matches} matchs, {o.stats.rounds} rounds
      {o.stats.skippedRounds > 0 && ` (${o.stats.skippedRounds} écartés : vainqueur inconnu)`}. Fenêtre de trade :{" "}
      {o.tradeWindowMs / 1000} s. Les pourcentages en gris reposent sur moins de 10 cas.
    </p>
  );
}
