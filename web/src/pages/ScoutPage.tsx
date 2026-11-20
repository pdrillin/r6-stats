import { useParams } from "react-router-dom";
import { useApi, type MatchSummary, type Overview, type Player, type Team } from "../api";
import { TimingChart } from "../components/charts";
import { Loading } from "../components/Loading";
import { FLink, MapFilter, scope, useMapFilter, useMatchFilter, useOverview } from "../components/MapFilter";
import { clutchTotal, playerPath, teamPath } from "../components/PlayerTable";
import { RateBars } from "../components/RateBars";
import { RoomCoverageNote, roomCoverage } from "../components/RoomCoverage";
import { Table } from "../components/Table";
import { Ratio, RecCount, RecRatio, date, rate } from "../format";
import { results, useMyTeam } from "../team";
import { Situations } from "./TeamsPage";

/** What a team needs to know before playing this one. */
export function ScoutPage() {
  const { name = "" } = useParams();
  const overview = useOverview();
  const matches = useApi<MatchSummary[]>("matches");
  const [map] = useMapFilter();
  const keep = useMatchFilter();
  return (
    <Loading state={overview}>
      {(o) => (
        <Loading state={matches}>
          {(all) => {
            const t = o.stats.teams.find((x) => x.name === name);
            return (
              <>
                <h1>
                  Fiche adversaire : {name}
                  <FLink to={teamPath(name)} className="compare-link">
                    Toutes ses stats
                  </FLink>
                </h1>
                <MapFilter maps={o.maps} />
                {!map && <p className="muted">Choisir une map pour préparer un match précis : sites, pièces et rythme y deviennent bien plus parlants.</p>}
                {t ? (
                  <Scout o={o} t={t} matches={all.filter(keep)} />
                ) : (
                  <p className="muted">Aucun match de {name}{scope(o)}.</p>
                )}
              </>
            );
          }}
        </Loading>
      )}
    </Loading>
  );
}

function Scout({ o, t, matches }: { o: Overview; t: Team; matches: MatchSummary[] }) {
  const [mine] = useMyTeam();
  const players = o.stats.players.filter((p) => p.team === t.name);
  const h2h = mine && mine !== t.name ? results(matches, mine).filter((r) => r.opponent === t.name) : [];
  const maps = [...new Set((t.sites ?? []).map((s) => s.map))].sort();
  const played = matches.filter((m) => m.teams.includes(t.name)).length;
  const sum = (get: (p: Player) => number[]) => players.reduce<number[]>((acc, p) => get(p).map((v, i) => v + (acc[i] ?? 0)), []);

  return (
    <>
      <p className="muted summary">
        Sur {t.rounds.count} rounds ({played} match{played > 1 ? "s" : ""}). Les pourcentages en
        gris reposent sur moins de 10 cas : à lire comme des tendances.
      </p>

      {h2h.length > 0 && (
        <>
          <h2>Nos matchs contre eux</h2>
          <ul className="result-list">
            {h2h.map((r) => (
              <li key={r.match.id}>
                <span className={`outcome ${r.outcome}`}>{r.outcome === "win" ? "V" : r.outcome === "loss" ? "D" : "N"}</span>
                <FLink to={"/matches/" + encodeURIComponent(r.match.id)} className="result-main">
                  <b>
                    {r.own}–{r.against}
                  </b>{" "}
                  <span className="muted">· {r.match.map}</span>
                </FLink>
                <span className="muted">{date(r.match.date)}</span>
              </li>
            ))}
          </ul>
        </>
      )}

      <h2>Leurs maps</h2>
      <RateBars
        label="Rounds gagnés par map"
        rows={Object.entries(t.byMap)
          .sort((a, b) => b[1].rounds.count - a[1].rounds.count)
          .map(([m, l]) => ({ key: m, label: m, n: l.rounds.won, of: l.rounds.count }))}
      />

      <h2>En défense : quel site ils choisissent</h2>
      <p className="muted">Part des rounds de défense passés sur chaque site, et rounds gagnés quand ils y défendent.</p>
      {maps.map((m) => {
        const sites = (t.sites ?? []).filter((s) => s.map === m && s.defense.count > 0);
        const total = sites.reduce((n, s) => n + s.defense.count, 0);
        if (total === 0) return null;
        return (
          <div key={m} className="map-block">
            <h3>{m}</h3>
            <Table
              rows={sites}
              rowKey={(s) => s.site}
              initialSort={1}
              columns={[
                { label: "Site", text: true, sort: (s) => s.site, render: (s) => s.site },
                { label: "Choisi", sort: (s) => s.defense.count, render: (s) => <Ratio n={s.defense.count} of={total} /> },
                { label: "Gagnés", sort: (s) => rate(s.defense.won, s.defense.count), render: (s) => <RecRatio r={s.defense} /> },
              ]}
            />
          </div>
        );
      })}

      <h2>En attaque : sur chaque site</h2>
      <p className="muted">Le site est imposé par la défense. Rounds gagnés, et rounds où ils posent le défuseur.</p>
      {maps.map((m) => {
        const sites = (t.sites ?? []).filter((s) => s.map === m && s.attack.count > 0);
        if (sites.length === 0) return null;
        return (
          <div key={m} className="map-block">
            <h3>{m}</h3>
            <Table
              rows={sites}
              rowKey={(s) => s.site}
              initialSort={1}
              columns={[
                { label: "Site", text: true, sort: (s) => s.site, render: (s) => s.site },
                { label: "Rounds", sort: (s) => s.attack.count, render: (s) => s.attack.count },
                { label: "Gagnés", sort: (s) => rate(s.attack.won, s.attack.count), render: (s) => <RecRatio r={s.attack} /> },
                { label: "Pose", sort: (s) => rate(s.plants.count, s.attack.count), render: (s) => <Ratio n={s.plants.count} of={s.attack.count} /> },
                { label: "Gagnés après pose", sort: (s) => rate(s.plants.won, s.plants.count), render: (s) => <RecRatio r={s.plants} /> },
              ]}
            />
          </div>
        );
      })}

      <h2>Joueurs à surveiller</h2>
      <p className="muted">Qui ouvre les rounds, qui meurt en premier, qui gagne les fins de round.</p>
      <Table
        rows={players}
        rowKey={(p) => p.id}
        initialSort={2}
        columns={[
          { label: "Joueur", text: true, sort: (p) => p.name.toLowerCase(), render: (p) => <FLink to={playerPath(p)}>{p.name}</FLink> },
          { label: "Rounds", sort: (p) => p.rounds, render: (p) => p.rounds },
          {
            label: "Premier kill",
            title: "Rounds où il fait le premier kill, et rounds gagnés ensuite",
            sort: (p) => rate(p.openingKills.count, p.rounds),
            render: (p) => <RecCount r={p.openingKills} />,
          },
          {
            label: "Première mort",
            sort: (p) => rate(p.openingDeaths.count, p.rounds),
            render: (p) => <RecCount r={p.openingDeaths} />,
          },
          { label: "KOST", sort: (p) => rate(p.kost, p.rounds), render: (p) => <Ratio n={p.kost} of={p.rounds} /> },
          { label: "Kills de trade", sort: (p) => p.tradeKills, render: (p) => p.tradeKills },
          { label: "Clutches", sort: (p) => clutchTotal(p).count, render: (p) => <RecRatio r={clutchTotal(p)} /> },
        ]}
      />

      <h2>Opérateurs</h2>
      <p className="muted">Les plus joués par chacun, avec le nombre de rounds.</p>
      <Operators players={players} sides={o.operatorSides} />

      <h2>Rythme</h2>
      <p className="muted">Quand ils tuent et quand ils meurent, par tranche de 30 secondes.</p>
      <TimingChart kills={sum((p) => p.killTiming)} deaths={sum((p) => p.deathTiming)} />

      {o.map ? (
        <>
          <h2>Pièces sur {o.map}</h2>
          <RoomCoverageNote players={players} />
          {roomCoverage(players).known > 0 && (
            <div className="two-cols">
              <section>
                <h3>Où ils tuent</h3>
                <RoomList players={players} get={(p) => p.killRooms} />
              </section>
              <section>
                <h3>Où ils meurent</h3>
                <RoomList players={players} get={(p) => p.deathRooms} />
              </section>
            </div>
          )}
        </>
      ) : (
        <p className="muted">Choisir une map pour voir les pièces où ils tuent et meurent.</p>
      )}

      <h2>Avantages numériques</h2>
      <p className="muted">Convertissent-ils leurs avantages, et reviennent-ils quand ils sont en infériorité ?</p>
      <Situations teams={[t]} />
    </>
  );
}

function Operators({ players, sides }: { players: Player[]; sides: Overview["operatorSides"] }) {
  const top = (p: Player, side: "attack" | "defense") =>
    Object.entries(p.byOperator ?? {})
      .filter(([op]) => sides[op] === side)
      .sort((a, b) => b[1].rounds - a[1].rounds)
      .slice(0, 3);
  const cell = (ops: ReturnType<typeof top>) =>
    ops.length === 0 ? (
      <span className="muted">–</span>
    ) : (
      ops.map(([op, l], i) => (
        <span key={op}>
          {i > 0 && ", "}
          {op} <span className="muted">{l.rounds}</span>
        </span>
      ))
    );
  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th className="text">Joueur</th>
            <th className="text">Attaque</th>
            <th className="text">Défense</th>
          </tr>
        </thead>
        <tbody>
          {players.map((p) => (
            <tr key={p.id}>
              <td className="text">{p.name}</td>
              <td className="text">{cell(top(p, "attack"))}</td>
              <td className="text">{cell(top(p, "defense"))}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function RoomList({ players, get }: { players: Player[]; get: (p: Player) => Record<string, number> }) {
  const total = new Map<string, number>();
  for (const p of players) for (const [room, n] of Object.entries(get(p) ?? {})) total.set(room, (total.get(room) ?? 0) + n);
  const rows = [...total.entries()].sort((a, b) => b[1] - a[1]).slice(0, 8);
  if (rows.length === 0) return <p className="muted">Aucune donnée.</p>;
  return (
    <div className="table-wrap">
      <table>
        <tbody>
          {rows.map(([room, n]) => (
            <tr key={room}>
              <td className="text">{room}</td>
              <td>{n}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
