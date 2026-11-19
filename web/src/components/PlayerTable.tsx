import type { Player, Rec } from "../api";
import { Ratio, RecCount, RecRatio, diff, rate } from "../format";
import { FLink } from "./MapFilter";
import { Table, type Column } from "./Table";

export function playerPath(p: { id: string }): string {
  return "/players/" + encodeURIComponent(p.id);
}

export function teamPath(name: string): string {
  return "/teams/" + encodeURIComponent(name);
}

/** All clutch sizes added up. */
export function clutchTotal(p: Player): Rec {
  const t = { count: 0, won: 0 };
  for (const r of Object.values(p.clutches ?? {})) {
    t.count += r.count;
    t.won += r.won;
  }
  return t;
}

const columns: Column<Player>[] = [
  {
    label: "Joueur",
    text: true,
    sort: (p) => p.name.toLowerCase(),
    render: (p) => <FLink to={playerPath(p)}>{p.name}</FLink>,
  },
  {
    label: "Équipe",
    // Cards are grouped by team already.
    noCard: true,
    text: true,
    sort: (p) => p.team,
    render: (p) => <FLink to={teamPath(p.team)}>{p.team}</FLink>,
  },
  { label: "Rounds", sort: (p) => p.rounds, render: (p) => p.rounds },
  { label: "Kills", title: "Adversaires tués (un coéquipier tué ne compte pas).", sort: (p) => p.kills, render: (p) => p.kills },
  { label: "Morts", sort: (p) => p.deaths, render: (p) => p.deaths },
  {
    label: "+/−",
    title: "Kills moins morts.",
    sort: (p) => p.kills - p.deaths,
    render: (p) => <span className={p.kills - p.deaths >= 0 ? "pos" : "neg"}>{diff(p.kills - p.deaths)}</span>,
  },
  {
    label: "Assists",
    title: "Aides à un kill, lues dans le tableau des scores du jeu.",
    extra: true,
    sort: (p) => p.assists,
    render: (p) => p.assists,
  },
  {
    label: "Headshots",
    title: "Part de ses kills faits d'une balle dans la tête.",
    extra: true,
    sort: (p) => rate(p.headshots, p.kills),
    render: (p) => <Ratio n={p.headshots} of={p.kills} />,
  },
  {
    label: "KOST",
    title:
      "Part des rounds où le joueur a été utile : il a fait un kill, posé ou désamorcé le défuseur, survécu, ou sa mort a été vengée. Un bon indicateur de régularité.",
    sort: (p) => rate(p.kost, p.rounds),
    render: (p) => <Ratio n={p.kost} of={p.rounds} />,
  },
  {
    label: "Survie",
    title: "Part des rounds terminés en vie.",
    extra: true,
    sort: (p) => rate(p.survived, p.rounds),
    render: (p) => <Ratio n={p.survived} of={p.rounds} />,
  },
  {
    label: "Premier kill",
    title: "Nombre de rounds où il fait le premier kill du round, et part de ces rounds que l'équipe gagne ensuite.",
    sort: (p) => p.openingKills.count,
    render: (p) => <RecCount r={p.openingKills} />,
  },
  {
    label: "Première mort",
    title: "Nombre de rounds où il meurt le premier, et part de ces rounds que l'équipe gagne quand même.",
    sort: (p) => p.openingDeaths.count,
    render: (p) => <RecCount r={p.openingDeaths} />,
  },
  {
    label: "Kills de vengeance",
    title: "Kills sur un adversaire qui venait de tuer un coéquipier, dans les 5 secondes (un « trade »).",
    extra: true,
    sort: (p) => p.tradeKills,
    render: (p) => p.tradeKills,
  },
  {
    label: "Morts vengées",
    title: "Part de ses morts vengées par un coéquipier dans les 5 secondes : un joueur souvent vengé ne meurt pas isolé.",
    extra: true,
    sort: (p) => rate(p.tradedDeaths, p.deaths),
    render: (p) => <Ratio n={p.tradedDeaths} of={p.deaths} />,
  },
  {
    label: "Clutches",
    title: "Rounds où il est resté le dernier en vie de son équipe, et part qu'il a gagnée.",
    sort: (p) => clutchTotal(p).count,
    render: (p) => <RecRatio r={clutchTotal(p)} />,
  },
  {
    label: "Poses",
    title: "Défuseurs posés, et part de ces rounds gagnés ensuite.",
    extra: true,
    sort: (p) => p.plants.count,
    render: (p) => <RecRatio r={p.plants} />,
  },
  {
    label: "Désamorçages",
    title: "Défuseurs désamorcés en défense.",
    extra: true,
    sort: (p) => p.defuses,
    render: (p) => p.defuses || "–",
  },
];

export function PlayerTable({ players, byTeam = true }: { players: Player[]; byTeam?: boolean }) {
  return (
    <Table
      rows={players}
      columns={columns}
      rowKey={(p) => p.id}
      initialSort={3}
      id="players"
      group={byTeam ? (p) => p.team : undefined}
    />
  );
}
