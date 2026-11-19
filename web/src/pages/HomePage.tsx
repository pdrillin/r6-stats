import type { ReactNode } from "react";
import { useApi, type MatchSummary, type Overview, type Player, type Team } from "../api";
import { Loading } from "../components/Loading";
import { FLink, MapFilter, scope, useMatchFilter, useOverview } from "../components/MapFilter";
import { clutchTotal, playerPath } from "../components/PlayerTable";
import { RateBars } from "../components/RateBars";
import { Table } from "../components/Table";
import { Ratio, RecCount, RecRatio, date, diff, rate, sideLabel } from "../format";
import { results, scoutPath, siteSides, useMyTeam } from "../team";

/** Below this many rounds a site is left out of strengths and weaknesses. */
const MIN_SITE_ROUNDS = 3;

export function HomePage() {
  const overview = useOverview();
  const matches = useApi<MatchSummary[]>("matches");
  const [mine, setMine, teamError] = useMyTeam();
  const keep = useMatchFilter();
  return (
    <Loading state={overview}>
      {(o) => (
        <Loading state={matches}>
          {(all) => {
            if (teamError) return <p className="error">Impossible d'enregistrer l'équipe : {teamError}</p>;
            if (mine === undefined) return <p className="muted">Chargement…</p>;
            const t = o.stats.teams.find((x) => x.name === mine);
            if (!mine) return <Picker o={o} onPick={setMine} />;
            return (
              <>
                <h1>
                  {mine}
                  <button className="link-button h1-action" onClick={() => setMine("")}>
                    Changer d'équipe
                  </button>
                </h1>
                <MapFilter maps={o.maps} />
                {t ? (
                  <Dashboard o={o} t={t} matches={all.filter(keep)} />
                ) : (
                  <p className="muted">Aucun match de {mine}{scope(o)}.</p>
                )}
              </>
            );
          }}
        </Loading>
      )}
    </Loading>
  );
}

function Picker({ o, onPick }: { o: Overview; onPick: (team: string) => void }) {
  return (
    <>
      <h1>Mon équipe</h1>
      <p className="muted">
        Choisis ton équipe : cette page devient son tableau de bord. Le choix est enregistré avec tes autres données
        (types des matchs) sur cet ordinateur.
      </p>
      <div className="team-picker">
        {o.stats.teams.map((t) => (
          <button key={t.name} onClick={() => onPick(t.name)}>
            <b>{t.name}</b>
            <span className="muted">{t.rounds.count} rounds joués</span>
          </button>
        ))}
      </div>
    </>
  );
}

function Dashboard({ o, t, matches }: { o: Overview; t: Team; matches: MatchSummary[] }) {
  const res = results(matches, t.name);
  const wins = res.filter((r) => r.outcome === "win").length;
  const losses = res.filter((r) => r.outcome === "loss").length;
  const players = o.stats.players.filter((p) => p.team === t.name);
  const attack = t.bySide["attack"]?.rounds ?? { count: 0, won: 0 };
  const defense = t.bySide["defense"]?.rounds ?? { count: 0, won: 0 };

  const sites = siteSides(t).filter((s) => s.count >= MIN_SITE_ROUNDS);
  const byRate = [...sites].sort((a, b) => b.won / b.count - a.won / a.count || b.count - a.count);
  const strong = byRate.filter((s) => s.won / s.count > 0.5).slice(0, 3);
  const weak = byRate.filter((s) => s.won / s.count < 0.5).reverse().slice(0, 3);

  // Opponents, most played first.
  const opponents = new Map<string, { wins: number; losses: number; played: number }>();
  for (const r of res) {
    const e = opponents.get(r.opponent) ?? { wins: 0, losses: 0, played: 0 };
    e.played++;
    if (r.outcome === "win") e.wins++;
    if (r.outcome === "loss") e.losses++;
    opponents.set(r.opponent, e);
  }

  return (
    <>
      <div className="cards">
        <Tile label="Matchs">
          {wins} V – {losses} D
        </Tile>
        <Tile label="Rounds gagnés">
          <RecRatio r={t.rounds} />
        </Tile>
        <Tile label="En attaque">
          <RecRatio r={attack} />
        </Tile>
        <Tile label="En défense">
          <RecRatio r={defense} />
        </Tile>
        <Tile label="Premier kill" hint="rounds où on l'obtient">
          <Ratio n={t.openingKills.count} of={t.rounds.count} />
        </Tile>
        <Tile label="Gagnés après le premier kill">
          <RecRatio r={t.openingKills} />
        </Tile>
        <Tile label="Gagnés après la première mort">
          <RecRatio r={t.openingDeaths} />
        </Tile>
        <Tile label="Morts tradées">
          <Ratio n={t.tradedDeaths} of={t.deaths} />
        </Tile>
      </div>

      <div className="two-cols">
        <section>
          <h2>Derniers matchs</h2>
          <ul className="result-list">
            {[...res]
              .reverse()
              .slice(0, 6)
              .map((r) => (
                <li key={r.match.id}>
                  <span className={`outcome ${r.outcome}`}>{r.outcome === "win" ? "V" : r.outcome === "loss" ? "D" : "N"}</span>
                  <FLink to={"/matches/" + encodeURIComponent(r.match.id)} className="result-main">
                    <b>
                      {r.own}–{r.against}
                    </b>{" "}
                    contre {r.opponent} <span className="muted">· {r.match.map}</span>
                  </FLink>
                  <span className="muted">{date(r.match.date)}</span>
                </li>
              ))}
          </ul>
        </section>
        <section>
          <h2>Adversaires</h2>
          <ul className="result-list">
            {[...opponents.entries()]
              .sort((a, b) => b[1].played - a[1].played)
              .map(([name, e]) => (
                <li key={name}>
                  <span className="result-main">
                    <b>{name}</b>{" "}
                    <span className="muted">
                      {e.wins} V – {e.losses} D
                    </span>
                  </span>
                  <FLink to={scoutPath(name)}>Préparer le prochain match →</FLink>
                </li>
              ))}
          </ul>
        </section>
      </div>

      <h2>Rounds gagnés par map</h2>
      <RateBars
        label="Rounds gagnés par map"
        rows={Object.entries(t.byMap)
          .sort((a, b) => b[1].rounds.count - a[1].rounds.count)
          .map(([m, l]) => ({ key: m, label: m, n: l.rounds.won, of: l.rounds.count }))}
      />

      <div className="two-cols">
        <section>
          <h2>Points forts</h2>
          <SiteList sites={strong} empty="Aucun site gagné plus d'une fois sur deux." />
        </section>
        <section>
          <h2>À travailler</h2>
          <SiteList sites={weak} empty="Aucun site perdu plus d'une fois sur deux." />
        </section>
      </div>
      <p className="muted">
        Sites joués au moins {MIN_SITE_ROUNDS} fois d'un même côté. Le détail par site est sur la{" "}
        <FLink to={"/teams/" + encodeURIComponent(t.name)}>page de l'équipe</FLink>.
      </p>

      <h2>Joueurs</h2>
      <PlayersCompact players={players} />
    </>
  );
}

function Tile({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
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

function SiteList({ sites, empty }: { sites: ReturnType<typeof siteSides>; empty: string }) {
  if (sites.length === 0) return <p className="muted">{empty}</p>;
  return (
    <RateBars
      label="Sites"
      rows={sites.map((s) => ({
        key: s.map + s.site + s.side,
        label: (
          <>
            {s.site}
            <span className="rate-sub">
              {sideLabel(s.side)} · {s.map}
            </span>
          </>
        ),
        n: s.won,
        of: s.count,
      }))}
    />
  );
}

/** The players' key figures, per round so they compare across rounds played. */
function PlayersCompact({ players }: { players: Player[] }) {
  const perRound = (n: number, r: number) => (r > 0 ? (n / r).toFixed(2).replace(".", ",") : "–");
  return (
    <Table
      rows={players}
      rowKey={(p) => p.id}
      initialSort={2}
      columns={[
        { label: "Joueur", text: true, sort: (p) => p.name.toLowerCase(), render: (p) => <FLink to={playerPath(p)}>{p.name}</FLink> },
        { label: "Rounds", sort: (p) => p.rounds, render: (p) => p.rounds },
        { label: "KOST", sort: (p) => rate(p.kost, p.rounds), render: (p) => <Ratio n={p.kost} of={p.rounds} /> },
        { label: "K / round", sort: (p) => rate(p.kills, p.rounds), render: (p) => perRound(p.kills, p.rounds) },
        { label: "D / round", sort: (p) => rate(p.deaths, p.rounds), render: (p) => perRound(p.deaths, p.rounds) },
        {
          label: "+/−",
          sort: (p) => p.kills - p.deaths,
          render: (p) => <span className={p.kills >= p.deaths ? "pos" : "neg"}>{diff(p.kills - p.deaths)}</span>,
        },
        { label: "Premier kill", sort: (p) => p.openingKills.count, render: (p) => <RecCount r={p.openingKills} /> },
        { label: "Première mort", sort: (p) => p.openingDeaths.count, render: (p) => <RecCount r={p.openingDeaths} /> },
        { label: "Clutches", sort: (p) => clutchTotal(p).count, render: (p) => <RecRatio r={clutchTotal(p)} /> },
      ]}
    />
  );
}
