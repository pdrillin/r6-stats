import { Link } from "react-router-dom";
import { useApi, type MatchSummary } from "../api";
import { Loading } from "../components/Loading";
import { MapFilter, useMapFilter } from "../components/MapFilter";
import { date } from "../format";

export function MatchesPage() {
  const state = useApi<MatchSummary[]>("matches");
  const [map] = useMapFilter();
  return (
    <Loading state={state}>
      {(all) => {
        const list = all.filter((m) => !map || m.map === map);
        return (
        <>
          <h1>Matchs</h1>
          <MapFilter maps={[...new Set(all.map((m) => m.map))].sort()} />
          {list.length === 0 && <p className="muted">Aucun match{map && ` sur ${map}`}.</p>}
          <ul className="match-list">
            {[...list].reverse().map((m) => (
              <li key={m.id}>
                <Link to={"/matches/" + encodeURIComponent(m.id)} className="match-card">
                  <span className="map">{m.map}</span>
                  <Score teams={m.teams} score={m.score} />
                  <span className="muted">
                    {date(m.date)} · {m.roundCount} rounds
                    {m.superseded > 0 && ` · rehost (${m.superseded} enregistrement${m.superseded > 1 ? "s" : ""} écarté${m.superseded > 1 ? "s" : ""})`}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </>
        );
      }}
    </Loading>
  );
}

export function Score({ teams, score }: { teams: [string, string]; score: [number, number] }) {
  const w = score[0] > score[1] ? 0 : score[1] > score[0] ? 1 : -1;
  return (
    <span className="score">
      <span className={w === 0 ? "win" : undefined}>{teams[0]}</span>
      <b>
        {score[0]}–{score[1]}
      </b>
      <span className={w === 1 ? "win" : undefined}>{teams[1]}</span>
    </span>
  );
}
