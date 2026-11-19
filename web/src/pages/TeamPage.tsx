import { useParams } from "react-router-dom";
import type { Overview, Team } from "../api";
import { TeamLineTable } from "../components/Breakdown";
import { TimingChart } from "../components/charts";
import { Loading } from "../components/Loading";
import { FLink, MapFilter, scope, useOverview } from "../components/MapFilter";
import { scoutPath } from "../team";
import { PlayerTable } from "../components/PlayerTable";
import { Table } from "../components/Table";
import { RecRatio, rate, sideLabel } from "../format";
import { Situations } from "./TeamsPage";

export function TeamPage() {
  const { name = "" } = useParams();
  const state = useOverview();
  return (
    <Loading state={state}>
      {(o) => {
        const t = o.stats.teams.find((x) => x.name === name);
        return t ? (
          <TeamView t={t} o={o} />
        ) : (
          <>
            <MapFilter maps={o.maps} />
            <p className="muted">Aucun match de cette équipe{scope(o)}.</p>
          </>
        );
      }}
    </Loading>
  );
}

function TeamView({ t, o }: { t: Team; o: Overview }) {
  const players = o.stats.players.filter((p) => p.team === t.name);
  const sum = (get: (p: (typeof players)[number]) => number[]) =>
    players.reduce<number[]>((acc, p) => get(p).map((v, i) => v + (acc[i] ?? 0)), []);
  return (
    <>
      <h1>
        {t.name} <span className="h1-sub">
          <RecRatio r={t.rounds} /> rounds gagnés
        </span>
      </h1>
      <MapFilter maps={o.maps} />
      <p>
        <FLink to={scoutPath(t.name)}>Fiche adversaire : préparer un match contre {t.name} →</FLink>
      </p>

      <h2>Moment du round</h2>
      <p className="muted">Kills et morts des joueurs de l'équipe par tranche de 30 secondes de la phase d'action.</p>
      <TimingChart kills={sum((p) => p.killTiming)} deaths={sum((p) => p.deathTiming)} />

      <h2>Par côté</h2>
      <TeamLineTable lines={t.bySide} label="Côté" keyLabel={sideLabel} />

      {!o.map && (
        <>
          <h2>Par map</h2>
          <TeamLineTable lines={t.byMap} label="Map" />
        </>
      )}

      <h2>Sites de bombe</h2>
      <p className="muted">Rounds gagnés en attaque et en défense sur chaque site ; poses : rounds d'attaque avec pose, et gagnés.</p>
      <Table
        rows={t.sites ?? []}
        rowKey={(s) => s.map + "|" + s.site}
        initialSort={0}
        group={(s) => s.map}
        columns={[
          { label: "Map", text: true, sort: (s) => s.map, render: (s) => s.map },
          { label: "Site", text: true, sort: (s) => s.site, render: (s) => s.site },
          { label: "Attaque", sort: (s) => rate(s.attack.won, s.attack.count), render: (s) => <RecRatio r={s.attack} /> },
          { label: "Poses", sort: (s) => rate(s.plants.count, s.attack.count), render: (s) => <RecRatio r={s.plants} /> },
          { label: "Défense", sort: (s) => rate(s.defense.won, s.defense.count), render: (s) => <RecRatio r={s.defense} /> },
        ]}
      />

      <h2>Avantages numériques</h2>
      <Situations teams={[t]} />

      <h2>Joueurs</h2>
      <PlayerTable players={players} byTeam={false} />
    </>
  );
}
