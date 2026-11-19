import { Link } from "react-router-dom";
import type { ReactNode } from "react";
import { useApi, type MatchSummary, type Overview, type Player, type Rec, type Team } from "../api";
import { Loading } from "../components/Loading";
import { FLink, MapFilter, scope, useMatchFilter, useOverview } from "../components/MapFilter";
import { clutchTotal, playerPath } from "../components/PlayerTable";
import { RateBars } from "../components/RateBars";
import { Table } from "../components/Table";
import { Ratio, RecCount, RecRatio, SMALL_SAMPLE, date, diff, percent, rate } from "../format";
import { insights, type Insight } from "../insights";
import { useWhere } from "../space";
import { results, scoutPath, useMyTeam } from "../team";

export function HomePage() {
  const overview = useOverview();
  const matches = useApi<MatchSummary[]>("matches");
  const [mine, setMine, teamError] = useMyTeam();
  const { server } = useWhere();
  const keep = useMatchFilter();
  return (
    <Loading state={overview}>
      {(o) => (
        <Loading state={matches}>
          {(all) => {
            if (teamError) return <p className="error">Impossible d'enregistrer l'équipe : {teamError}</p>;
            if (mine === undefined) return <p className="muted">Chargement…</p>;
            const checklist = server && <Checklist hasMatches={all.length > 0} hasTeam={!!mine} />;
            if (all.length === 0 || !mine) {
              return (
                <>
                  <h1>Bienvenue</h1>
                  {checklist}
                  {all.length > 0 && !mine && <Picker o={o} onPick={setMine} />}
                  {!server && all.length === 0 && <p className="muted">Aucun match pour l'instant.</p>}
                </>
              );
            }
            const t = o.stats.teams.find((x) => x.name === mine);
            const shown = all.filter(keep);
            return (
              <>
                <PageHead
                  team={mine}
                  o={o}
                  played={shown.filter((m) => m.teams.includes(mine)).length}
                  onChange={() => setMine("")}
                />
                {checklist}
                <MapFilter maps={o.maps} />
                {t ? (
                  <Dashboard o={o} t={t} matches={shown} />
                ) : (
                  <p className="muted">
                    Aucun match de {mine}
                    {scope(o)}.
                  </p>
                )}
              </>
            );
          }}
        </Loading>
      )}
    </Loading>
  );
}

function PageHead({ team, o, played, onChange }: { team: string; o: Overview; played: number; onChange: () => void }) {
  const { canEdit } = useWhere();
  const rounds = o.stats.teams.find((t) => t.name === team)?.rounds.count ?? 0;
  return (
    <div className="page-head">
      <h1>{team}</h1>
      <p className="lede">
        {played} match{played > 1 ? "s" : ""}, {rounds} rounds{scope(o)}
        {canEdit && (
          <>
            {" · "}
            <button className="link-button" onClick={onChange}>
              changer d'équipe
            </button>
          </>
        )}
      </p>
    </div>
  );
}

/**
 * What a new space still needs, until it is done: replays, the team the
 * space follows, and its members.
 */
function Checklist({ hasMatches, hasTeam }: { hasMatches: boolean; hasTeam: boolean }) {
  const { canEdit } = useWhere();
  const members = useApi<unknown[]>("members");
  const hasMembers = (members.data?.length ?? 0) > 1;
  if (members.data === undefined || (hasMatches && hasTeam && hasMembers)) return null;
  const steps: { done: boolean; title: string; action: ReactNode }[] = [
    {
      done: hasMatches,
      title: "Envoyer des replays",
      action: (
        <Link to="/upload" className="button primary small">
          Envoyer
        </Link>
      ),
    },
    {
      done: hasTeam,
      title: "Indiquer votre équipe",
      action: canEdit ? (
        <span className="muted">{hasMatches ? "Choisis-la ci-dessous" : "Après le premier envoi"}</span>
      ) : (
        <span className="muted">Un responsable s'en charge</span>
      ),
    },
    {
      done: hasMembers,
      title: "Inviter coéquipiers et coach",
      action: canEdit ? (
        <Link to="/members" className="button small">
          Inviter
        </Link>
      ) : (
        <span className="muted">Un responsable s'en charge</span>
      ),
    },
  ];
  return (
    <section className="checklist" aria-label="Pour bien démarrer">
      <h2>Pour bien démarrer</h2>
      <ol>
        {steps.map((s, i) => (
          <li key={s.title} className={s.done ? "done" : undefined}>
            <span className="step-mark" aria-hidden="true">
              {s.done ? "✓" : i + 1}
            </span>
            <span className="step-title">
              {s.title}
              {s.done && <span className="visually-hidden"> (fait)</span>}
            </span>
            {!s.done && s.action}
          </li>
        ))}
      </ol>
    </section>
  );
}

function Picker({ o, onPick }: { o: Overview; onPick: (team: string) => void }) {
  const { canEdit, server } = useWhere();
  if (!canEdit) {
    return (
      <p className="muted">
        Un responsable de l'espace doit indiquer quelle équipe est la vôtre : cette page deviendra son tableau de bord.
      </p>
    );
  }
  return (
    <section>
      <h2>Quelle est votre équipe ?</h2>
      <p className="muted">
        Cette page devient son tableau de bord.{" "}
        {server ? "Le choix vaut pour tous les membres de l'espace." : "Le choix est enregistré sur cet ordinateur."}
      </p>
      <div className="team-picker">
        {o.stats.teams.map((t) => (
          <button key={t.name} onClick={() => onPick(t.name)}>
            <b>{t.name}</b>
            <span className="muted">{t.rounds.count} rounds joués</span>
          </button>
        ))}
      </div>
    </section>
  );
}

function Dashboard({ o, t, matches }: { o: Overview; t: Team; matches: MatchSummary[] }) {
  const res = results(matches, t.name);
  const wins = res.filter((r) => r.outcome === "win").length;
  const losses = res.filter((r) => r.outcome === "loss").length;
  const players = o.stats.players.filter((p) => p.team === t.name);
  const zero = { count: 0, won: 0 };
  const found = insights(t, players);

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
      <div className="key-numbers">
        <KeyNumber label="Matchs" value={`${wins} – ${losses}`} sub="victoires – défaites" />
        <RateNumber label="Rounds gagnés" r={t.rounds} />
        <RateNumber label="En attaque" r={t.bySide["attack"]?.rounds ?? zero} />
        <RateNumber label="En défense" r={t.bySide["defense"]?.rounds ?? zero} />
      </div>

      <section>
        <h2>Ce qui ressort</h2>
        <Insights list={found} />
      </section>

      <div className="two-cols">
        <section>
          <h2>Derniers matchs</h2>
          <ul className="result-list">
            {[...res]
              .reverse()
              .slice(0, 5)
              .map((r) => (
                <li key={r.match.id}>
                  <span className={`outcome ${r.outcome}`}>
                    {r.outcome === "win" ? "V" : r.outcome === "loss" ? "D" : "N"}
                  </span>
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
          {res.length > 5 && (
            <p>
              <FLink to="/matches" className="small-link">
                Tous les matchs ({res.length})
              </FLink>
            </p>
          )}
        </section>
        <section>
          <h2>Préparer un match</h2>
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
                  <FLink to={scoutPath(name)}>Fiche adversaire →</FLink>
                </li>
              ))}
          </ul>
        </section>
      </div>

      <section>
        <h2>Rounds gagnés par map</h2>
        <RateBars
          label="Rounds gagnés par map"
          rows={Object.entries(t.byMap)
            .sort((a, b) => b[1].rounds.count - a[1].rounds.count)
            .map(([m, l]) => ({ key: m, label: m, n: l.rounds.won, of: l.rounds.count }))}
        />
      </section>

      <section>
        <h2>Joueurs</h2>
        <PlayersCompact players={players} />
      </section>
    </>
  );
}

function KeyNumber({ label, value, sub }: { label: string; value: ReactNode; sub: ReactNode }) {
  return (
    <div className="key-number">
      <div className="key-label">{label}</div>
      <div className="key-value">{value}</div>
      <div className="key-sub">{sub}</div>
    </div>
  );
}

/** A rate as a key number: the percentage, then the sample it rests on. */
function RateNumber({ label, r }: { label: string; r: Rec }) {
  if (r.count === 0) return <KeyNumber label={label} value="–" sub="aucun round" />;
  return (
    <KeyNumber
      label={label}
      value={percent(r.won, r.count)}
      sub={
        <>
          {r.won} rounds sur {r.count}
          {r.count < SMALL_SAMPLE && " · peu de rounds"}
        </>
      }
    />
  );
}

const toneMarks: Record<Insight["tone"], { mark: string; label: string }> = {
  good: { mark: "▲", label: "Point fort" },
  bad: { mark: "▼", label: "À travailler" },
  info: { mark: "●", label: "À savoir" },
};

function Insights({ list }: { list: Insight[] }) {
  if (list.length === 0) {
    return (
      <p className="muted">
        Pas encore assez de rounds pour dégager des tendances : il en faut au moins 5 dans une situation pour qu'elle
        apparaisse ici.
      </p>
    );
  }
  return (
    <ul className="insights">
      {list.map((i) => (
        <li key={i.key} className={i.tone}>
          <span className="insight-mark" role="img" aria-label={toneMarks[i.tone].label} title={toneMarks[i.tone].label}>
            {toneMarks[i.tone].mark}
          </span>
          <span>{i.text}</span>
        </li>
      ))}
    </ul>
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
        {
          label: "Joueur",
          text: true,
          sort: (p) => p.name.toLowerCase(),
          render: (p) => <FLink to={playerPath(p)}>{p.name}</FLink>,
        },
        { label: "Rounds", sort: (p) => p.rounds, render: (p) => p.rounds },
        {
          label: "KOST",
          title: "Rounds où le joueur a été utile : kill, pose ou désamorçage, survie, ou mort vengée",
          sort: (p) => rate(p.kost, p.rounds),
          render: (p) => <Ratio n={p.kost} of={p.rounds} />,
        },
        { label: "Kills / round", sort: (p) => rate(p.kills, p.rounds), render: (p) => perRound(p.kills, p.rounds) },
        { label: "Morts / round", sort: (p) => rate(p.deaths, p.rounds), render: (p) => perRound(p.deaths, p.rounds) },
        {
          label: "+/−",
          title: "Kills moins morts",
          sort: (p) => p.kills - p.deaths,
          render: (p) => <span className={p.kills >= p.deaths ? "pos" : "neg"}>{diff(p.kills - p.deaths)}</span>,
        },
        {
          label: "Premier kill",
          title: "Rounds où il fait le premier kill, et part de ces rounds gagnés",
          sort: (p) => p.openingKills.count,
          render: (p) => <RecCount r={p.openingKills} />,
        },
        {
          label: "Clutches",
          title: "Rounds où il est resté seul contre un ou plusieurs adversaires, et part gagnée",
          sort: (p) => clutchTotal(p).count,
          render: (p) => <RecRatio r={clutchTotal(p)} />,
        },
      ]}
    />
  );
}
