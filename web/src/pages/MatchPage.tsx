import { useEffect, useRef, useState, type ReactNode } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { kindLabels, useApi, useSend, type MatchDetail, type MatchKind, type Round } from "../api";
import { useWhere } from "../space";
import { RoundTimeline } from "../components/charts";
import { Loading } from "../components/Loading";
import { PlayerTable, playerPath, teamPath } from "../components/PlayerTable";
import { Help } from "../components/Table";
import { TabList, TabPanel, useTab } from "../components/Tabs";
import { Ratio, RecCount, RecRatio, clock, date, percent, sideLabel, winReasons } from "../format";
import { Situations } from "./TeamsPage";

export function MatchPage() {
  const { id = "" } = useParams();
  const state = useApi<MatchDetail>("matches/" + encodeURIComponent(id));
  return <Loading state={state}>{(m) => <Match m={m} />}</Loading>;
}

const TABS = [
  { id: "resume", label: "Résumé" },
  { id: "rounds", label: "Rounds" },
  { id: "joueurs", label: "Joueurs" },
];

function Match({ m }: { m: MatchDetail }) {
  const [tab, setTab] = useTab(TABS);
  // Rounds unfolded in the Rounds tab, by number.
  const [open, setOpen] = useState<Set<number>>(new Set());
  const goToRound = (n: number) => {
    setOpen(new Set([n]));
    setTab("rounds");
    // Once the tab is shown, bring the round into view.
    requestAnimationFrame(() => document.getElementById(`round-${n}`)?.scrollIntoView({ block: "center" }));
  };
  return (
    <>
      <header className="match-head">
        <div className="match-title">
          <h1>{m.map}</h1>
          <MatchMenu m={m} />
        </div>
        <div className="match-score">
          <TeamScore name={m.teams[0]} score={m.score[0]} team={0} won={m.score[0] > m.score[1]} />
          <span className="match-dash" aria-hidden="true">
            –
          </span>
          <TeamScore name={m.teams[1]} score={m.score[1]} team={1} won={m.score[1] > m.score[0]} right />
        </div>
        <p className="lede">
          {date(m.date)} · {m.roundCount} rounds
          {m.superseded > 0 &&
            ` · partie rehostée : ${m.superseded} enregistrement${m.superseded > 1 ? "s" : ""} de round interrompu écarté${m.superseded > 1 ? "s" : ""}`}
        </p>
        <KindPicker m={m} />
      </header>

      <TabList tabs={TABS} current={tab} onChange={setTab} label="Vues du match" />

      {tab === "resume" && (
        <TabPanel id="resume">
          <h2>Déroulé</h2>
          <p className="muted">Un carré par round, à la couleur du vainqueur : il mène au détail du round.</p>
          <Progress m={m} onPick={goToRound} />
          <TeamCompare m={m} />
          <h2>Chronologie des kills</h2>
          <p className="muted">Chaque point est un kill, placé au moment où il a lieu. Survoler ou toucher un point pour le détail.</p>
          <RoundTimeline m={m} />
        </TabPanel>
      )}

      {tab === "rounds" && (
        <TabPanel id="rounds">
          <div className="tab-tools">
            <span className="muted">Ouvrir un round montre son kill feed et les opérateurs joués.</span>
            <button
              className="link-button"
              onClick={() => setOpen(open.size === m.rounds.length ? new Set() : new Set(m.rounds.map((r) => r.number)))}
            >
              {open.size === m.rounds.length ? "Tout replier" : "Tout déplier"}
            </button>
          </div>
          <div className="rounds">
            {m.rounds.map((r) => (
              <RoundRow
                key={r.number}
                m={m}
                r={r}
                open={open.has(r.number)}
                onToggle={() => {
                  const next = new Set(open);
                  if (next.has(r.number)) next.delete(r.number);
                  else next.add(r.number);
                  setOpen(next);
                }}
              />
            ))}
          </div>
        </TabPanel>
      )}

      {tab === "joueurs" && (
        <TabPanel id="joueurs">
          <PlayerTable players={m.stats.players} />
          <h2>Avantages numériques</h2>
          <p className="muted">
            Rounds passés par chaque situation (joueurs en vie de chaque côté), et part gagnée par l'équipe.
          </p>
          <Situations teams={m.stats.teams} />
        </TabPanel>
      )}
    </>
  );
}

function TeamScore({ name, score, team, won, right }: { name: string; score: number; team: number; won: boolean; right?: boolean }) {
  return (
    <span className={["team-score", right ? "right" : "", won ? "won" : ""].join(" ")}>
      <Link to={teamPath(name)} className={`tk${team}`}>
        {name}
      </Link>
      <b>{score}</b>
    </span>
  );
}

/** The two teams side by side on what decided the match. */
function TeamCompare({ m }: { m: MatchDetail }) {
  const teams = m.teams.map((name) => m.stats.teams.find((t) => t.name === name));
  if (!teams[0] || !teams[1]) return null;
  const zero = { count: 0, won: 0 };
  // Plants, from the rounds: the attacking team planted or not.
  const plants = [0, 1].map((t) => {
    const attacked = m.rounds.filter((r) => r.attacker === t);
    const planted = attacked.filter((r) => r.plant);
    return { rounds: attacked.length, planted: planted.length, won: planted.filter((r) => r.winner === t).length };
  });
  const clutches = [0, 1].map((t) => {
    const total = { count: 0, won: 0 };
    for (const p of m.stats.players.filter((p) => p.team === m.teams[t])) {
      for (const c of Object.values(p.clutches ?? {})) {
        total.count += c.count;
        total.won += c.won;
      }
    }
    return total;
  });
  const best = [0, 1].map(
    (t) =>
      m.stats.players
        .filter((p) => p.team === m.teams[t])
        .sort((a, b) => b.kost - a.kost || b.kills - a.kills)[0],
  );
  const rows: { label: string; help?: string; cells: [ReactNode, ReactNode] }[] = [
    {
      label: "En attaque",
      cells: [0, 1].map((t) => <RecRatio key={t} r={teams[t]!.bySide["attack"]?.rounds ?? zero} />) as [ReactNode, ReactNode],
    },
    {
      label: "En défense",
      cells: [0, 1].map((t) => <RecRatio key={t} r={teams[t]!.bySide["defense"]?.rounds ?? zero} />) as [ReactNode, ReactNode],
    },
    {
      label: "Premier kill",
      help: "Rounds où l'équipe fait le premier kill, et part de ces rounds gagnés ensuite.",
      cells: [0, 1].map((t) => <RecCount key={t} r={teams[t]!.openingKills} />) as [ReactNode, ReactNode],
    },
    {
      label: "Défuseur posé",
      help: "Rounds d'attaque où l'équipe pose le défuseur, et rounds gagnés après la pose.",
      cells: plants.map((p, t) =>
        p.rounds === 0 ? (
          <span key={t} className="muted">
            –
          </span>
        ) : (
          <span key={t}>
            {p.planted} sur {p.rounds}
            {p.planted > 0 && <span className="sample"> · {percent(p.won, p.planted)} gagnés</span>}
          </span>
        ),
      ) as [ReactNode, ReactNode],
    },
    {
      label: "Clutches",
      help: "Rounds où un joueur est resté le dernier en vie de l'équipe, et part gagnée.",
      cells: clutches.map((c, t) => <RecRatio key={t} r={c} />) as [ReactNode, ReactNode],
    },
    {
      label: "Morts vengées",
      help: "Part des morts vengées par un coéquipier dans les 5 secondes.",
      cells: [0, 1].map((t) => <Ratio key={t} n={teams[t]!.tradedDeaths} of={teams[t]!.deaths} />) as [ReactNode, ReactNode],
    },
    {
      label: "Le plus régulier",
      help: "Meilleur KOST de l'équipe : part des rounds avec un kill, une pose ou un désamorçage, une survie, ou une mort vengée.",
      cells: best.map((p, t) =>
        p ? (
          <span key={t}>
            <Link to={playerPath(p)}>{p.name}</Link> <span className="sample">{percent(p.kost, p.rounds)} · {p.kills} kills</span>
          </span>
        ) : (
          <span key={t}>–</span>
        ),
      ) as [ReactNode, ReactNode],
    },
  ];
  return (
    <section>
      <h2>Les deux équipes</h2>
      <div className="table-wrap">
        <table className="compare-table">
          <thead>
            <tr>
              <th className="text">
                <span className="visually-hidden">Statistique</span>
              </th>
              <th className="text">
                <span className="tk0">{m.teams[0]}</span>
              </th>
              <th className="text">
                <span className="tk1">{m.teams[1]}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.label}>
                <th scope="row" className="text">
                  <span className="th-inner">
                    {row.label}
                    {row.help && <Help label={row.label} text={row.help} />}
                  </span>
                </th>
                <td className="text">{row.cells[0]}</td>
                <td className="text">{row.cells[1]}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

/**
 * Rare actions on the match, out of the way: a "⋯" menu. For now, deleting
 * the match, for managers of a space.
 */
function MatchMenu({ m }: { m: MatchDetail }) {
  const { server, canEdit } = useWhere();
  const send = useSend();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string>();
  const menu = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (e: Event) => {
      if (e instanceof KeyboardEvent ? e.key === "Escape" : !menu.current?.contains(e.target as Node)) setOpen(false);
    };
    window.addEventListener("keydown", close);
    window.addEventListener("pointerdown", close);
    return () => {
      window.removeEventListener("keydown", close);
      window.removeEventListener("pointerdown", close);
    };
  }, [open]);
  if (!server || !canEdit) return null;
  const remove = async () => {
    setOpen(false);
    if (
      !confirm(
        `Supprimer ce match (${m.map}, ${m.teams[0]} ${m.score[0]}–${m.score[1]} ${m.teams[1]}) de l'espace ? Ses rounds et ses statistiques disparaissent pour tous les membres.`,
      )
    ) {
      return;
    }
    try {
      await send("DELETE", `matches/${encodeURIComponent(m.id)}`);
      navigate("/matches");
    } catch (err) {
      setError(String(err));
    }
  };
  return (
    <div className="menu" ref={menu}>
      <button
        className="menu-trigger"
        aria-label="Autres actions"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        ⋯
      </button>
      {open && (
        <div className="menu-items" role="menu">
          <button role="menuitem" className="danger-item" onClick={remove}>
            Supprimer ce match…
          </button>
        </div>
      )}
      {error && <p className="error">{error}</p>}
    </div>
  );
}

/** The kind of the match, which only the user can tell. */
function KindPicker({ m }: { m: MatchDetail }) {
  const send = useSend();
  const { canEdit } = useWhere();
  const [error, setError] = useState<string>();
  const [saving, setSaving] = useState(false);
  const change = (kind: MatchKind) => {
    setSaving(true);
    setError(undefined);
    send("PUT", `matches/${encodeURIComponent(m.id)}/kind`, { kind })
      .catch((e: unknown) => setError(String(e)))
      .finally(() => setSaving(false));
  };
  if (!canEdit) {
    return (
      <div className="filters">
        <span className="muted">Type de match</span>
        <span className={`kind-badge ${m.kind}`}>{kindLabels[m.kind]}</span>
      </div>
    );
  }
  return (
    <div className="filters">
      <span className="muted" id="kind-label">
        Type de match
      </span>
      <div className="tabs segmented" role="radiogroup" aria-labelledby="kind-label">
        {(Object.keys(kindLabels) as MatchKind[]).map((k) => (
          <button
            key={k}
            role="radio"
            aria-checked={m.kind === k}
            className={m.kind === k ? "active" : undefined}
            disabled={saving}
            onClick={() => m.kind !== k && change(k)}
          >
            {kindLabels[k]}
          </button>
        ))}
      </div>
      {error && <span className="error">Échec de l'enregistrement : {error}</span>}
    </div>
  );
}

/** One square per round, in the winner's color, leading to the round. */
function Progress({ m, onPick }: { m: MatchDetail; onPick: (round: number) => void }) {
  return (
    <div className="progress-block">
      <div className="progress" role="list" aria-label="Rounds">
        {m.rounds.map((r) => {
          const who = r.winner >= 0 ? m.teams[r.winner] : "vainqueur inconnu";
          return (
            <button
              key={r.number}
              role="listitem"
              className={`dot t${r.winner}`}
              title={`Round ${r.number} : ${who}`}
              aria-label={`Round ${r.number}, gagné par ${who}`}
              onClick={() => onPick(r.number)}
            >
              {r.number}
            </button>
          );
        })}
      </div>
      <div className="legend">
        <span className="legend-item">
          <span className="swatch" />
          {m.teams[0]}
        </span>
        <span className="legend-item">
          <span className="swatch t1-swatch" />
          {m.teams[1]}
        </span>
      </div>
    </div>
  );
}

function RoundRow({ m, r, open, onToggle }: { m: MatchDetail; r: Round; open: boolean; onToggle: () => void }) {
  const team = (i: number) => (i === 0 || i === 1 ? m.teams[i] : "?");
  return (
    <div className={open ? "round open" : "round"} id={`round-${r.number}`}>
      <button className="round-head" onClick={onToggle} aria-expanded={open}>
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
