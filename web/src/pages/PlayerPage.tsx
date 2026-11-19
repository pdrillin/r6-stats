import { useState, type ReactNode } from "react";
import { Link, useParams } from "react-router-dom";
import { useApi, type Overview, type Player } from "../api";
import { LineTable } from "../components/Breakdown";
import { TimingChart } from "../components/charts";
import { Loading } from "../components/Loading";
import { teamPath } from "../components/PlayerTable";
import { Table } from "../components/Table";
import { Ratio, RecCount, diff, sideLabel } from "../format";

export function PlayerPage() {
  const { id = "" } = useParams();
  const state = useApi<Overview>("overview");
  return (
    <Loading state={state}>
      {(o) => {
        const p = o.stats.players.find((x) => x.id === id);
        return p ? <PlayerView p={p} /> : <p className="error">Joueur introuvable.</p>;
      }}
    </Loading>
  );
}

const splits = {
  side: { label: "Côté", lines: (p: Player) => p.bySide, key: sideLabel },
  map: { label: "Map", lines: (p: Player) => p.byMap, key: undefined },
  operator: { label: "Opérateur", lines: (p: Player) => p.byOperator, key: undefined },
};

function PlayerView({ p }: { p: Player }) {
  const [split, setSplit] = useState<keyof typeof splits>("side");
  const s = splits[split];
  const clutchSizes = Object.keys(p.clutches ?? {}).sort((a, b) => Number(a) - Number(b));
  return (
    <>
      <h1>
        {p.name} <Link to={teamPath(p.team)} className="team-link">{p.team}</Link>
      </h1>
      <div className="cards">
        <Card label="Rounds">{p.rounds}</Card>
        <Card label="K / D / A">
          {p.kills} / {p.deaths} / {p.assists} <span className={p.kills >= p.deaths ? "pos" : "neg"}>({diff(p.kills - p.deaths)})</span>
        </Card>
        <Card label="KOST"><Ratio n={p.kost} of={p.rounds} /></Card>
        <Card label="Survie"><Ratio n={p.survived} of={p.rounds} /></Card>
        <Card label="Headshots"><Ratio n={p.headshots} of={p.kills} /></Card>
        <Card label="Premier kill"><RecCount r={p.openingKills} /></Card>
        <Card label="Première mort"><RecCount r={p.openingDeaths} /></Card>
        <Card label="Morts tradées"><Ratio n={p.tradedDeaths} of={p.deaths} /></Card>
        <Card label="Kills de trade">{p.tradeKills}</Card>
        <Card label="Poses"><RecCount r={p.plants} /></Card>
        <Card label="Désamorçages">{p.defuses}</Card>
        <Card label="Clutches" hint="gagnés / joués">
          {clutchSizes.length === 0 ? (
            <span className="muted">aucun</span>
          ) : (
            clutchSizes.map((x) => (
              <div key={x}>
                1v{x} : {p.clutches[x]!.won}/{p.clutches[x]!.count}
              </div>
            ))
          )}
        </Card>
      </div>

      <h2>Moment du round</h2>
      <p className="muted">Kills et morts par tranche de 30 secondes de la phase d'action.</p>
      <TimingChart kills={p.killTiming} deaths={p.deathTiming} />

      <h2>
        Par{" "}
        <span className="tabs">
          {(Object.keys(splits) as (keyof typeof splits)[]).map((k) => (
            <button key={k} className={k === split ? "active" : undefined} onClick={() => setSplit(k)}>
              {splits[k].label.toLowerCase()}
            </button>
          ))}
        </span>
      </h2>
      {split === "operator" && (
        <p className="muted">Les rounds dont l'en-tête ne donne pas l'opérateur du joueur ne figurent pas ici.</p>
      )}
      <LineTable key={split} lines={s.lines(p)} label={s.label} keyLabel={s.key} />

      <h2>Pièces</h2>
      <p className="muted">Kills comptés dans la pièce où se trouvait le joueur, morts dans celle où il est tombé.</p>
      <RoomTable p={p} />
    </>
  );
}

function Card({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <div className="card">
      <div className="card-label">
        {label}
        {hint && <span className="hint"> · {hint}</span>}
      </div>
      <div className="card-value">{children}</div>
    </div>
  );
}

function RoomTable({ p }: { p: Player }) {
  const names = new Set([...Object.keys(p.killRooms ?? {}), ...Object.keys(p.deathRooms ?? {})]);
  const rows = [...names].map((room) => ({ room, kills: p.killRooms[room] ?? 0, deaths: p.deathRooms[room] ?? 0 }));
  return (
    <Table
      rows={rows}
      rowKey={(r) => r.room}
      initialSort={1}
      columns={[
        { label: "Pièce", text: true, sort: (r) => r.room, render: (r) => r.room },
        { label: "Kills", sort: (r) => r.kills, render: (r) => r.kills || "–" },
        { label: "Morts", sort: (r) => r.deaths, render: (r) => r.deaths || "–" },
      ]}
    />
  );
}
