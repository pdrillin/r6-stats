import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useApi, type MatchDetail, type Round } from "../api";
import { RoundTimeline } from "../components/charts";
import { Loading } from "../components/Loading";
import { PlayerTable, teamPath } from "../components/PlayerTable";
import { clock, date, sideLabel, winReasons } from "../format";
import { Score } from "./MatchesPage";
import { Situations } from "./TeamsPage";

export function MatchPage() {
  const { id = "" } = useParams();
  const state = useApi<MatchDetail>("matches/" + encodeURIComponent(id));
  return <Loading state={state}>{(m) => <Match m={m} />}</Loading>;
}

function Match({ m }: { m: MatchDetail }) {
  return (
    <>
      <h1>
        {m.map} <Score teams={m.teams} score={m.score} />
      </h1>
      <p className="muted summary">
        {date(m.date)} · {m.roundCount} rounds
        {m.superseded > 0 &&
          ` · partie rehostée : ${m.superseded} enregistrement${m.superseded > 1 ? "s" : ""} de round interrompu écarté${m.superseded > 1 ? "s" : ""}`}
      </p>
      <Progress m={m} />

      <h2>Chronologie</h2>
      <p className="muted">Chaque point est un kill, placé au moment où il a lieu. Survoler un point pour le détail.</p>
      <RoundTimeline m={m} />

      <h2>Rounds</h2>
      <p className="muted">Cliquer sur un round pour voir le kill feed et les opérateurs.</p>
      <div className="rounds">
        {m.rounds.map((r) => (
          <RoundRow key={r.number} m={m} r={r} />
        ))}
      </div>

      <h2>Joueurs</h2>
      <PlayerTable players={m.stats.players} />

      <h2>Avantages numériques</h2>
      <Situations teams={m.stats.teams} />
    </>
  );
}

/** One square per round, in the winner's color. */
function Progress({ m }: { m: MatchDetail }) {
  return (
    <div className="progress" aria-label="Rounds gagnés">
      {m.rounds.map((r) => (
        <span
          key={r.number}
          className={`dot t${r.winner}`}
          title={`Round ${r.number} : ${r.winner >= 0 ? m.teams[r.winner] : "vainqueur inconnu"}`}
        >
          {r.number}
        </span>
      ))}
    </div>
  );
}

function RoundRow({ m, r }: { m: MatchDetail; r: Round }) {
  const [open, setOpen] = useState(false);
  const team = (i: number) => (i === 0 || i === 1 ? m.teams[i] : "?");
  return (
    <div className={open ? "round open" : "round"}>
      <button className="round-head" onClick={() => setOpen(!open)} aria-expanded={open}>
        <span className="num">R{r.number}</span>
        <span className="muted">
          {r.score[0]}–{r.score[1]}
        </span>
        <span>
          <span className={`tag tk${r.attacker}`}>{team(r.attacker)}</span> attaque
        </span>
        <span className="site">{r.site || "site inconnu"}</span>
        <span>
          <span className={`tag tk${r.winner}`}>{team(r.winner)}</span> {winReasons[r.winReason].toLowerCase()}
          {r.defuser && ` (${r.defuser})`}
        </span>
        <span className="muted">
          {r.plant ? `pose ${clock(r.plant.elapsedMs)}${r.planter ? ` par ${r.planter}` : ""}` : ""}
        </span>
        <span className="muted">
          {r.kills.length} kills · {clock(r.end.elapsedMs)}
        </span>
      </button>
      {open && <RoundDetail m={m} r={r} />}
    </div>
  );
}

function RoundDetail({ m, r }: { m: MatchDetail; r: Round }) {
  return (
    <div className="round-body">
      <ol className="feed">
        {r.kills.map((k, i) => (
          <li key={i}>
            <span className="time">{clock(k.elapsedMs)}</span>
            <span className={`tk${k.killerTeam}`}>{k.killer || "?"}</span>
            {k.killerRoom && <span className="room">{k.killerRoom}</span>}
            <span className="muted">{k.headshot ? "⌖ HS" : "→"}</span>
            <span className={`tk${k.victimTeam}`}>{k.victim}</span>
            {k.victimRoom && <span className="room">{k.victimRoom}</span>}
            {k.killerTeam === k.victimTeam && <span className="neg">team kill</span>}
          </li>
        ))}
        {r.kills.length === 0 && <li className="muted">Aucun kill.</li>}
      </ol>
      <div className="lineups">
        {[0, 1].map((t) => (
          <table key={t} className="lineup">
            <thead>
              <tr>
                <th className="text">
                  <Link to={teamPath(m.teams[t]!)} className={`tk${t}`}>
                    {m.teams[t]}
                  </Link>{" "}
                  <span className="muted">{sideLabel(r.players.find((p) => p.team === t)?.side ?? "")}</span>
                </th>
                <th className="text">Opérateur</th>
                <th>K</th>
                <th>D</th>
                <th>A</th>
              </tr>
            </thead>
            <tbody>
              {r.players
                .filter((p) => p.team === t)
                .map((p) => (
                  <tr key={p.name} className={p.deaths > 0 ? "dead" : undefined}>
                    <td className="text">{p.name}</td>
                    <td className="text">{p.operator || <span className="muted">inconnu</span>}</td>
                    <td>{p.kills}</td>
                    <td>{p.deaths}</td>
                    <td>{p.assists}</td>
                  </tr>
                ))}
            </tbody>
          </table>
        ))}
      </div>
    </div>
  );
}
