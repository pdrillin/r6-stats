import { Link } from "react-router-dom";
import type { Player, Rec } from "../api";
import { Ratio, RecCount, RecRatio, diff, rate } from "../format";
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
    render: (p) => <Link to={playerPath(p)}>{p.name}</Link>,
  },
  {
    label: "Équipe",
    text: true,
    sort: (p) => p.team,
    render: (p) => <Link to={teamPath(p.team)}>{p.team}</Link>,
  },
  { label: "Rounds", sort: (p) => p.rounds, render: (p) => p.rounds },
  { label: "K", title: "Kills (ennemis seulement)", sort: (p) => p.kills, render: (p) => p.kills },
  { label: "D", title: "Morts", sort: (p) => p.deaths, render: (p) => p.deaths },
  {
    label: "+/−",
    sort: (p) => p.kills - p.deaths,
    render: (p) => <span className={p.kills - p.deaths >= 0 ? "pos" : "neg"}>{diff(p.kills - p.deaths)}</span>,
  },
  { label: "A", title: "Assists, lus dans le tableau des scores du jeu", sort: (p) => p.assists, render: (p) => p.assists },
  {
    label: "HS",
    title: "Part des kills en headshot",
    sort: (p) => rate(p.headshots, p.kills),
    render: (p) => <Ratio n={p.headshots} of={p.kills} />,
  },
  {
    label: "KOST",
    title: "Rounds avec un kill, une pose ou un désamorçage, une survie ou une mort tradée",
    sort: (p) => rate(p.kost, p.rounds),
    render: (p) => <Ratio n={p.kost} of={p.rounds} />,
  },
  {
    label: "Survie",
    title: "Rounds terminés en vie",
    sort: (p) => rate(p.survived, p.rounds),
    render: (p) => <Ratio n={p.survived} of={p.rounds} />,
  },
  {
    label: "Ouv. K",
    title: "Premier kill du round : nombre, et rounds gagnés ensuite",
    sort: (p) => p.openingKills.count,
    render: (p) => <RecCount r={p.openingKills} />,
  },
  {
    label: "Ouv. D",
    title: "Première mort du round : nombre, et rounds gagnés quand même",
    sort: (p) => p.openingDeaths.count,
    render: (p) => <RecCount r={p.openingDeaths} />,
  },
  {
    label: "Trades",
    title: "Kills qui vengent un coéquipier tué dans les 5 secondes",
    sort: (p) => p.tradeKills,
    render: (p) => p.tradeKills,
  },
  {
    label: "Morts tradées",
    title: "Morts vengées par un coéquipier dans les 5 secondes",
    sort: (p) => rate(p.tradedDeaths, p.deaths),
    render: (p) => <Ratio n={p.tradedDeaths} of={p.deaths} />,
  },
  {
    label: "Clutches",
    title: "Rounds où le joueur est resté seul, et rounds gagnés",
    sort: (p) => clutchTotal(p).count,
    render: (p) => <RecRatio r={clutchTotal(p)} />,
  },
  {
    label: "Poses",
    title: "Défuseurs posés, et rounds gagnés ensuite",
    sort: (p) => p.plants.count,
    render: (p) => <RecRatio r={p.plants} />,
  },
  { label: "Désam.", title: "Défuseurs désamorcés", sort: (p) => p.defuses, render: (p) => p.defuses || "–" },
];

export function PlayerTable({ players, byTeam = true }: { players: Player[]; byTeam?: boolean }) {
  return (
    <Table
      rows={players}
      columns={columns}
      rowKey={(p) => p.id}
      initialSort={3}
      group={byTeam ? (p) => p.team : undefined}
    />
  );
}
