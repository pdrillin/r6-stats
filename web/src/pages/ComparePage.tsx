import type { ReactNode } from "react";
import { useSearchParams } from "react-router-dom";
import type { Player } from "../api";
import { LegendItem, TimingChart } from "../components/charts";
import { Loading } from "../components/Loading";
import { FLink, MapFilter, useOverview } from "../components/MapFilter";
import { clutchTotal, playerPath, teamPath } from "../components/PlayerTable";
import { SMALL_SAMPLE, percent, sideLabel } from "../format";

export function ComparePage() {
  const state = useOverview();
  const [params, setParams] = useSearchParams();
  const pick = (key: "a" | "b", id: string) =>
    setParams(
      (p) => {
        p.set(key, id);
        return p;
      },
      { replace: true },
    );
  return (
    <Loading state={state}>
      {(o) => {
        const players = [...o.stats.players].sort((x, y) => x.team.localeCompare(y.team) || x.name.localeCompare(y.name));
        const find = (id: string | null) => players.find((p) => p.id === id);
        // Without a choice, the first two players of the list.
        const aId = params.get("a") ?? players[0]?.id ?? null;
        const bId = params.get("b") ?? players.find((p) => p.id !== aId)?.id ?? null;
        const a = find(aId);
        const b = find(bId);
        return (
          <>
            <h1>Comparer deux joueurs</h1>
            <MapFilter maps={o.maps}>
              <PlayerSelect label="Joueur A" color="var(--blue)" players={players} value={aId} onChange={(id) => pick("a", id)} />
              <PlayerSelect label="Joueur B" color="var(--orange)" players={players} value={bId} onChange={(id) => pick("b", id)} />
            </MapFilter>
            {!a || !b ? (
              <p className="muted">
                {o.map ? `Un des deux joueurs n'a pas joué sur ${o.map}.` : "Choisir deux joueurs."}
              </p>
            ) : a.id === b.id ? (
              <p className="muted">Choisir deux joueurs différents.</p>
            ) : (
              <Comparison a={a} b={b} />
            )}
          </>
        );
      }}
    </Loading>
  );
}

function PlayerSelect({
  label,
  color,
  players,
  value,
  onChange,
}: {
  label: string;
  color: string;
  players: Player[];
  value: string | null;
  onChange: (id: string) => void;
}) {
  const teams = [...new Set(players.map((p) => p.team))];
  return (
    <label>
      <LegendItem color={color}>
        <span className="muted">{label}</span>
      </LegendItem>
      <select value={value ?? ""} onChange={(e) => onChange(e.target.value)}>
        {value && !players.some((p) => p.id === value) && <option value={value}>(absent de cette map)</option>}
        {teams.map((t) => (
          <optgroup key={t} label={t}>
            {players
              .filter((p) => p.team === t)
              .map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
          </optgroup>
        ))}
      </select>
    </label>
  );
}

/** A value to compare: n out of `of`, shown as a rate. */
interface Stat {
  n: number;
  of: number;
  /** How to print the rate. */
  format: "percent" | "perRound";
}

interface Row {
  label: string;
  title?: string;
  a: Stat;
  b: Stat;
  /** For deaths and the like, the lower rate is the better one. */
  lowerIsBetter?: boolean;
}

function rows(a: Player, b: Player): { section: string; rows: Row[] }[] {
  const per = (get: (p: Player) => number): [Stat, Stat] => [
    { n: get(a), of: a.rounds, format: "perRound" },
    { n: get(b), of: b.rounds, format: "perRound" },
  ];
  const pct = (n: (p: Player) => number, of: (p: Player) => number): [Stat, Stat] => [
    { n: n(a), of: of(a), format: "percent" },
    { n: n(b), of: of(b), format: "percent" },
  ];
  const row = (label: string, [sa, sb]: [Stat, Stat], extra: Partial<Row> = {}): Row => ({ label, a: sa, b: sb, ...extra });
  const side = (p: Player, s: string) => p.bySide[s] ?? { rounds: 0, kost: 0, won: 0, kills: 0, deaths: 0 };
  const sideRows = ["attack", "defense"].flatMap((s) => [
    row(`KOST en ${sideLabel(s).toLowerCase()}`, pct((p) => side(p, s).kost, (p) => side(p, s).rounds)),
    row(`Kills par round en ${sideLabel(s).toLowerCase()}`, [
      { n: side(a, s).kills, of: side(a, s).rounds, format: "perRound" },
      { n: side(b, s).kills, of: side(b, s).rounds, format: "perRound" },
    ]),
  ]);
  return [
    {
      section: "Combat",
      rows: [
        row("Kills par round", per((p) => p.kills)),
        row("Morts par round", per((p) => p.deaths), { lowerIsBetter: true }),
        row("Assists par round", per((p) => p.assists)),
        row("Headshots", pct((p) => p.headshots, (p) => p.kills), { title: "Part des kills en headshot" }),
      ],
    },
    {
      section: "Impact",
      rows: [
        row("KOST", pct((p) => p.kost, (p) => p.rounds), {
          title: "Rounds avec un kill, une pose ou un désamorçage, une survie ou une mort tradée",
        }),
        row("Survie", pct((p) => p.survived, (p) => p.rounds)),
        row("Premier kill", pct((p) => p.openingKills.count, (p) => p.rounds), { title: "Rounds où le joueur fait le premier kill" }),
        row("Rounds gagnés après son premier kill", pct((p) => p.openingKills.won, (p) => p.openingKills.count)),
        row("Première mort", pct((p) => p.openingDeaths.count, (p) => p.rounds), {
          lowerIsBetter: true,
          title: "Rounds où le joueur meurt le premier",
        }),
        row("Kills de trade par round", per((p) => p.tradeKills), { title: "Kills qui vengent un coéquipier tué dans les 5 secondes" }),
        row("Morts tradées", pct((p) => p.tradedDeaths, (p) => p.deaths), { title: "Morts vengées par un coéquipier dans les 5 secondes" }),
        row("Clutches gagnés", pct((p) => clutchTotal(p).won, (p) => clutchTotal(p).count)),
      ],
    },
    { section: "Par côté", rows: sideRows },
    {
      section: "Objectif",
      rows: [
        row("Poses par round", per((p) => p.plants.count)),
        row("Désamorçages par round", per((p) => p.defuses)),
      ],
    },
  ];
}

const rate = (s: Stat) => (s.of > 0 ? s.n / s.of : null);

function show(s: Stat): string {
  if (s.of === 0) return "–";
  if (s.format === "percent") return percent(s.n, s.of);
  return (s.n / s.of).toFixed(2).replace(".", ",");
}

function sample(s: Stat): string {
  if (s.of === 0) return "aucun cas";
  return s.format === "percent" ? `${s.n}/${s.of}` : `${s.n} en ${s.of} rds`;
}

function Comparison({ a, b }: { a: Player; b: Player }) {
  const scale = Math.max(1, ...a.killTiming, ...a.deathTiming, ...b.killTiming, ...b.deathTiming);
  return (
    <>
      <div className="versus-head">
        <PlayerHead p={a} color="var(--blue)" />
        <span className="muted">contre</span>
        <PlayerHead p={b} color="var(--orange)" right />
      </div>
      <p className="muted">
        Valeurs ramenées au nombre de rounds joués par chacun. La valeur la plus favorable est en gras ; les barres
        comparent les deux joueurs ligne par ligne. En gris : moins de {SMALL_SAMPLE} cas.
      </p>

      {rows(a, b).map((s) => (
        <section key={s.section}>
          <h2>{s.section}</h2>
          <div className="versus">
            {s.rows.map((r) => (
              <VersusRow key={r.label} r={r} />
            ))}
          </div>
        </section>
      ))}

      <h2>Moment du round</h2>
      <p className="muted">
        Kills et morts par tranche de 30 secondes, sur la même échelle.
        {a.rounds !== b.rounds &&
          ` Ce sont des totaux, et les nombres de rounds diffèrent : ${a.name} ${a.rounds}, ${b.name} ${b.rounds}.`}
      </p>
      <div className="small-multiples">
        <div>
          <h3>
            <LegendItem color="var(--blue)">{a.name}</LegendItem>
          </h3>
          <TimingChart kills={a.killTiming} deaths={a.deathTiming} max={scale} table={false} />
        </div>
        <div>
          <h3>
            <LegendItem color="var(--orange)">{b.name}</LegendItem>
          </h3>
          <TimingChart kills={b.killTiming} deaths={b.deathTiming} max={scale} table={false} />
        </div>
      </div>
      <p className="muted">
        Le détail chiffré est sur la page de chaque joueur : <FLink to={playerPath(a)}>{a.name}</FLink>,{" "}
        <FLink to={playerPath(b)}>{b.name}</FLink>.
      </p>
    </>
  );
}

function PlayerHead({ p, color, right }: { p: Player; color: string; right?: boolean }) {
  return (
    <div className={right ? "versus-player right" : "versus-player"}>
      <LegendItem color={color}>
        <FLink to={playerPath(p)} className="versus-name">
          {p.name}
        </FLink>
      </LegendItem>
      <span className="muted">
        <FLink to={teamPath(p.team)}>{p.team}</FLink> · {p.rounds} rounds
      </span>
    </div>
  );
}

function VersusRow({ r }: { r: Row }) {
  const ra = rate(r.a);
  const rb = rate(r.b);
  const top = Math.max(ra ?? 0, rb ?? 0);
  let better: "a" | "b" | null = null;
  if (ra !== null && rb !== null && ra !== rb) better = (ra > rb) !== !!r.lowerIsBetter ? "a" : "b";
  const width = (v: number | null) => (v === null || top === 0 ? 0 : (v / top) * 100);
  return (
    <div className="versus-row" title={r.title}>
      <Value s={r.a} strong={better === "a"} />
      <div className="versus-bar left">
        <span style={{ width: `${width(ra)}%`, background: "var(--blue)" }} />
      </div>
      <div className="versus-label">
        {r.label}
        {r.lowerIsBetter && <span className="muted"> (moins = mieux)</span>}
      </div>
      <div className="versus-bar">
        <span style={{ width: `${width(rb)}%`, background: "var(--orange)" }} />
      </div>
      <Value s={r.b} strong={better === "b"} right />
    </div>
  );
}

function Value({ s, strong, right }: { s: Stat; strong: boolean; right?: boolean }): ReactNode {
  const small = s.format === "percent" && s.of < SMALL_SAMPLE;
  return (
    <div className={["versus-value", right ? "right" : "", small ? "small" : ""].join(" ")}>
      <span className={strong ? "best" : undefined}>{show(s)}</span> <span className="sample">{sample(s)}</span>
    </div>
  );
}
