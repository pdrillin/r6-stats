import type { Team } from "../api";
import { openingColumns } from "../components/Breakdown";
import { Loading } from "../components/Loading";
import { FLink, MapFilter, useOverview } from "../components/MapFilter";
import { teamPath } from "../components/PlayerTable";
import { Table, type Column } from "../components/Table";
import { Ratio, RecRatio, rate } from "../format";
import { Summary } from "./OverviewPage";

const columns: Column<Team>[] = [
  { label: "Équipe", text: true, sort: (t) => t.name, render: (t) => <FLink to={teamPath(t.name)}>{t.name}</FLink> },
  { label: "Rounds gagnés", sort: (t) => rate(t.rounds.won, t.rounds.count), render: (t) => <RecRatio r={t.rounds} /> },
  {
    label: "Attaque",
    sort: (t) => rateOf(t.bySide["attack"]?.rounds),
    render: (t) => <RecRatio r={t.bySide["attack"]?.rounds ?? zero} />,
  },
  {
    label: "Défense",
    sort: (t) => rateOf(t.bySide["defense"]?.rounds),
    render: (t) => <RecRatio r={t.bySide["defense"]?.rounds ?? zero} />,
  },
  ...openingColumns((t: Team) => ({ rounds: t.rounds.count, kills: t.openingKills, deaths: t.openingDeaths })),
  {
    label: "Morts vengées",
    title: "Part des morts vengées par un coéquipier dans les 5 secondes.",
    sort: (t) => rate(t.tradedDeaths, t.deaths),
    render: (t) => <Ratio n={t.tradedDeaths} of={t.deaths} />,
  },
];

const zero = { count: 0, won: 0 };

function rateOf(r?: { count: number; won: number }) {
  return r ? rate(r.won, r.count) : null;
}

export function TeamsPage() {
  const state = useOverview();
  return (
    <Loading state={state}>
      {(o) => (
        <>
          <h1>Équipes</h1>
          <MapFilter maps={o.maps} />
          <Summary o={o} />
          <Table rows={o.stats.teams} columns={columns} rowKey={(t) => t.name} initialSort={1} />
          <h2>Avantages numériques</h2>
          <p className="muted">
            Rounds passés par chaque situation (joueurs en vie de l'équipe contre ceux d'en face), et part gagnée.
            Chaque situation compte une fois par round.
          </p>
          <Situations teams={o.stats.teams} />
        </>
      )}
    </Loading>
  );
}

export function Situations({ teams }: { teams: Team[] }) {
  const all = new Map<string, { own: number; enemy: number }>();
  for (const t of teams) for (const s of t.situations) all.set(`${s.own}v${s.enemy}`, s);
  const keys = [...all.values()].sort((a, b) => b.own - b.enemy - (a.own - a.enemy) || b.own - a.own);
  return (
    <div className="table-wrap">
      <table className="situations">
        <thead>
          <tr>
            <th className="text">Équipe</th>
            {keys.map((k) => (
              <th key={`${k.own}v${k.enemy}`} className={k.own > k.enemy ? "pos" : k.own < k.enemy ? "neg" : ""}>
                {k.own}v{k.enemy}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {teams.map((t) => (
            <tr key={t.name}>
              <td className="text">{t.name}</td>
              {keys.map((k) => {
                const s = t.situations.find((x) => x.own === k.own && x.enemy === k.enemy);
                return <td key={`${k.own}v${k.enemy}`}>{s ? <Ratio n={s.won} of={s.count} /> : <span className="muted">–</span>}</td>;
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
