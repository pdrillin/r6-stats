import type { Line, Rec, TeamLine } from "../api";
import { Ratio, RecRatio, diff, rate } from "../format";
import { Table, type Column } from "./Table";

type Row<L> = { key: string; line: L };

function rows<L>(m: Record<string, L>): Row<L>[] {
  return Object.entries(m ?? {}).map(([key, line]) => ({ key, line }));
}

/** A player's stats split by side, map or operator. */
export function LineTable({ lines, label, keyLabel }: { lines: Record<string, Line>; label: string; keyLabel?: (k: string) => string }) {
  const columns: Column<Row<Line>>[] = [
    { label, text: true, sort: (r) => r.key, render: (r) => (keyLabel ? keyLabel(r.key) : r.key) },
    { label: "Rounds", sort: (r) => r.line.rounds, render: (r) => r.line.rounds },
    {
      label: "Victoires",
      sort: (r) => rate(r.line.won, r.line.rounds),
      render: (r) => <Ratio n={r.line.won} of={r.line.rounds} />,
    },
    { label: "Kills", sort: (r) => r.line.kills, render: (r) => r.line.kills },
    { label: "Morts", sort: (r) => r.line.deaths, render: (r) => r.line.deaths },
    {
      label: "+/−",
      sort: (r) => r.line.kills - r.line.deaths,
      render: (r) => {
        const d = r.line.kills - r.line.deaths;
        return <span className={d >= 0 ? "pos" : "neg"}>{diff(d)}</span>;
      },
    },
    { label: "Assists", extra: true, sort: (r) => r.line.assists, render: (r) => r.line.assists },
    {
      label: "KOST",
      title: "Part des rounds où le joueur a été utile : kill, pose ou désamorçage, survie, ou mort vengée.",
      sort: (r) => rate(r.line.kost, r.line.rounds),
      render: (r) => <Ratio n={r.line.kost} of={r.line.rounds} />,
    },
    {
      label: "Survie",
      extra: true,
      sort: (r) => rate(r.line.survived, r.line.rounds),
      render: (r) => <Ratio n={r.line.survived} of={r.line.rounds} />,
    },
    {
      label: "Premier kill – première mort",
      title: "Nombre de rounds où il fait le premier kill, et nombre où il meurt le premier.",
      sort: (r) => r.line.openingKills - r.line.openingDeaths,
      render: (r) => `${r.line.openingKills}–${r.line.openingDeaths}`,
    },
    { label: "Poses", extra: true, sort: (r) => r.line.plants, render: (r) => r.line.plants || "–" },
    { label: "Désamorçages", extra: true, sort: (r) => r.line.defuses, render: (r) => r.line.defuses || "–" },
  ];
  return <Table rows={rows(lines)} columns={columns} rowKey={(r) => r.key} initialSort={1} id="line" />;
}

/**
 * How often a team got the first kill or lost the first player, and how
 * often it won those rounds: two separate questions, so two columns each.
 */
export function openingColumns<T>(get: (row: T) => { rounds: number; kills: Rec; deaths: Rec }): Column<T>[] {
  return [
    {
      label: "1er kill",
      title: "Rounds où l'équipe fait le premier kill",
      sort: (r) => rate(get(r).kills.count, get(r).rounds),
      render: (r) => <Ratio n={get(r).kills.count} of={get(r).rounds} />,
    },
    {
      label: "Gagnés après",
      title: "Parmi ces rounds, ceux que l'équipe gagne",
      sort: (r) => rate(get(r).kills.won, get(r).kills.count),
      render: (r) => <RecRatio r={get(r).kills} />,
    },
    {
      label: "1re mort",
      title: "Rounds où l'équipe perd le premier joueur",
      sort: (r) => rate(get(r).deaths.count, get(r).rounds),
      render: (r) => <Ratio n={get(r).deaths.count} of={get(r).rounds} />,
    },
    {
      label: "Gagnés quand même",
      title: "Parmi ces rounds, ceux que l'équipe gagne malgré tout",
      sort: (r) => rate(get(r).deaths.won, get(r).deaths.count),
      render: (r) => <RecRatio r={get(r).deaths} />,
    },
  ];
}

/** A team's stats split by side or map. */
export function TeamLineTable({ lines, label, keyLabel }: { lines: Record<string, TeamLine>; label: string; keyLabel?: (k: string) => string }) {
  const columns: Column<Row<TeamLine>>[] = [
    { label, text: true, sort: (r) => r.key, render: (r) => (keyLabel ? keyLabel(r.key) : r.key) },
    {
      label: "Rounds gagnés",
      sort: (r) => rate(r.line.rounds.won, r.line.rounds.count),
      render: (r) => <RecRatio r={r.line.rounds} />,
    },
    ...openingColumns((r: Row<TeamLine>) => ({
      rounds: r.line.rounds.count,
      kills: r.line.openingKills,
      deaths: r.line.openingDeaths,
    })),
    {
      label: "Morts vengées",
      title: "Part des morts vengées par un coéquipier dans les 5 secondes.",
      sort: (r) => rate(r.line.tradedDeaths, r.line.deaths),
      render: (r) => <Ratio n={r.line.tradedDeaths} of={r.line.deaths} />,
    },
  ];
  return <Table rows={rows(lines)} columns={columns} rowKey={(r) => r.key} initialSort={0} />;
}
