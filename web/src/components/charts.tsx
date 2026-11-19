// Charts drawn as plain SVG at the container's pixel width. Each one has a
// hover and focus tooltip and a table holding the same values, so nothing is
// only readable by hovering.

import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
import type { MatchDetail, Room } from "../api";
import { clock } from "../format";

/** Tracks an element's width. */
function useWidth<T extends HTMLElement>() {
  const ref = useRef<T>(null);
  const [width, setWidth] = useState(0);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    setWidth(el.clientWidth);
    const ro = new ResizeObserver((e) => setWidth(e[0]!.contentRect.width));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  return [ref, width] as const;
}

interface Tip {
  x: number;
  y: number;
  content: ReactNode;
}

/** A chart frame: positions the tooltip and hosts the legend. */
function Frame({
  legend,
  tip,
  children,
  frameRef,
  label,
}: {
  legend?: ReactNode;
  tip: Tip | null;
  children: ReactNode;
  frameRef: React.RefObject<HTMLDivElement | null>;
  label: string;
}) {
  return (
    <figure className="chart" aria-label={label}>
      {legend && <div className="legend">{legend}</div>}
      <div className="chart-plot" ref={frameRef}>
        {children}
        {tip && (
          <div
            className="tooltip"
            role="status"
            style={{
              left: tip.x,
              top: tip.y,
              transform: `translate(${tip.x > (frameRef.current?.clientWidth ?? 0) / 2 ? "calc(-100% - 12px)" : "12px"}, -50%)`,
            }}
          >
            {tip.content}
          </div>
        )}
      </div>
    </figure>
  );
}

/** Hover and keyboard handlers that show a tooltip for one mark. */
function hover(setTip: (t: Tip | null) => void, x: number, y: number, content: ReactNode) {
  return {
    tabIndex: 0,
    onPointerEnter: () => setTip({ x, y, content }),
    onPointerLeave: () => setTip(null),
    onFocus: () => setTip({ x, y, content }),
    onBlur: () => setTip(null),
  };
}

export function LegendItem({ color, shape = "rect", children }: { color: string; shape?: "rect" | "dot" | "ring" | "tick"; children: ReactNode }) {
  return (
    <span className="legend-item">
      <svg width="14" height="12" aria-hidden="true">
        {shape === "rect" && <rect x="1" y="1" width="12" height="10" rx="2" fill={color} />}
        {shape === "dot" && <circle cx="7" cy="6" r="4.5" fill={color} />}
        {shape === "ring" && <circle cx="7" cy="6" r="3.8" fill="none" stroke={color} strokeWidth="2" />}
        {shape === "tick" && <rect x="6" y="0" width="2" height="12" fill={color} />}
      </svg>
      {children}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Kill and death timing

export const timingLabels = ["Prépa", "0:00", "0:30", "1:00", "1:30", "2:00", "2:30", "3:00+"];

function timingRange(i: number): string {
  if (i === 0) return "Phase de préparation";
  if (i === timingLabels.length - 1) return "Après 3:00 (défuseur posé)";
  return `${timingLabels[i]} – ${timingLabels[i + 1]}`;
}

/**
 * Kills above the baseline, deaths below, by moment of the round. One axis:
 * both are counts of the same events.
 */
export function TimingChart({ kills, deaths }: { kills: number[]; deaths: number[] }) {
  const [ref, width] = useWidth<HTMLDivElement>();
  const [tip, setTip] = useState<Tip | null>(null);
  const n = timingLabels.length;
  const height = 220;
  const pad = { top: 16, bottom: 28, left: 34, right: 8 };
  const max = Math.max(1, ...kills, ...deaths);
  const step = niceStep(max);
  const top = Math.ceil(max / step) * step;
  const plotH = height - pad.top - pad.bottom;
  const zero = pad.top + plotH / 2;
  const y = (v: number) => (v / top) * (plotH / 2);
  const band = (width - pad.left - pad.right) / n;
  const barW = Math.min(24, band * 0.6);
  const ticks: number[] = [];
  for (let v = -top; v <= top; v += step) ticks.push(v);

  return (
    <>
      <Frame
        frameRef={ref}
        tip={tip}
        label="Kills et morts selon le moment du round"
        legend={
          <>
            <LegendItem color="var(--kills)">Kills</LegendItem>
            <LegendItem color="var(--deaths)">Morts</LegendItem>
          </>
        }
      >
        {width > 0 && (
          <svg width={width} height={height} role="img" aria-label="Histogramme des kills et des morts par tranche de 30 secondes">
            {ticks.map((v) => (
              <g key={v}>
                <line x1={pad.left} x2={width - pad.right} y1={zero - (v / top) * (plotH / 2)} y2={zero - (v / top) * (plotH / 2)} className={v === 0 ? "axis" : "grid"} />
                <text x={pad.left - 6} y={zero - (v / top) * (plotH / 2)} className="tick" textAnchor="end" dominantBaseline="middle">
                  {Math.abs(v)}
                </text>
              </g>
            ))}
            {timingLabels.map((label, i) => {
              const cx = pad.left + band * i + band / 2;
              const k = kills[i] ?? 0;
              const d = deaths[i] ?? 0;
              return (
                <g
                  key={label}
                  className="mark-group"
                  {...hover(setTip, cx, zero, (
                    <>
                      <div className="tip-title">{timingRange(i)}</div>
                      <TipRow color="var(--kills)" value={k} label="kills" />
                      <TipRow color="var(--deaths)" value={d} label="morts" />
                    </>
                  ))}
                >
                  <rect x={cx - band / 2} y={pad.top} width={band} height={plotH} fill="transparent" />
                  {k > 0 && <path d={bar(cx - barW / 2, zero - 1, barW, y(k) - 1, "up")} fill="var(--kills)" />}
                  {d > 0 && <path d={bar(cx - barW / 2, zero + 1, barW, y(d) - 1, "down")} fill="var(--deaths)" />}
                  <text x={cx} y={height - 8} className="tick" textAnchor="middle">
                    {label}
                  </text>
                </g>
              );
            })}
          </svg>
        )}
      </Frame>
      <details className="table-view">
        <summary>Voir en tableau</summary>
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th className="text">Moment</th>
                <th>Kills</th>
                <th>Morts</th>
              </tr>
            </thead>
            <tbody>
              {timingLabels.map((_, i) => (
                <tr key={i}>
                  <td className="text">{timingRange(i)}</td>
                  <td>{kills[i] ?? 0}</td>
                  <td>{deaths[i] ?? 0}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </details>
    </>
  );
}

function TipRow({ color, value, label }: { color: string; value: ReactNode; label: string }) {
  return (
    <div className="tip-row">
      <span className="tip-key" style={{ background: color }} />
      <b>{value}</b> <span className="muted">{label}</span>
    </div>
  );
}

/** A bar with a 4px rounded data end and a square base. */
function bar(x: number, base: number, w: number, len: number, dir: "up" | "down" | "right"): string {
  if (len <= 0) return "";
  const r = Math.min(4, len, w / 2);
  if (dir === "right") {
    const end = x + len;
    return `M${x},${base} H${end - r} Q${end},${base} ${end},${base + r} V${base + w - r} Q${end},${base + w} ${end - r},${base + w} H${x} Z`;
  }
  const s = dir === "up" ? -1 : 1;
  const end = base + s * len;
  return `M${x},${base} V${end - s * r} Q${x},${end} ${x + r},${end} H${x + w - r} Q${x + w},${end} ${x + w},${end - s * r} V${base} Z`;
}

/** A round tick step for an axis reaching max: 1, 2, 5, 10, 20… */
function niceStep(max: number): number {
  const raw = max / 3;
  const mag = 10 ** Math.floor(Math.log10(raw));
  for (const m of [1, 2, 5, 10]) if (m * mag >= raw) return Math.max(1, m * mag);
  return Math.max(1, 10 * mag);
}

// ---------------------------------------------------------------------------
// Match timeline

/**
 * One line per round, one dot per kill at its time, in the killer's team
 * color; the plant is a tick. Shows when fights happen and who wins them.
 */
export function RoundTimeline({ m }: { m: MatchDetail }) {
  const [ref, width] = useWidth<HTMLDivElement>();
  const [tip, setTip] = useState<Tip | null>(null);
  const rowH = 26;
  const pad = { top: 8, bottom: 26, left: 44, right: 56 };
  const prepW = 22; // the preparation phase, squeezed: kills there are rare
  const hasPrep = m.rounds.some((r) => r.kills.some((k) => k.phase === "prep" || k.elapsedMs < 0));
  const maxMs = Math.max(180000, ...m.rounds.map((r) => r.end.elapsedMs), ...m.rounds.flatMap((r) => r.kills.map((k) => k.elapsedMs)));
  const x0 = pad.left + (hasPrep ? prepW : 0);
  const x = (ms: number) => (ms < 0 ? pad.left + prepW / 2 : x0 + (ms / maxMs) * (width - x0 - pad.right));
  const height = pad.top + rowH * m.rounds.length + pad.bottom;
  const ticks: number[] = [];
  for (let t = 0; t <= maxMs; t += 30000) ticks.push(t);

  return (
    <Frame
      frameRef={ref}
      tip={tip}
      label="Chronologie des kills"
      legend={
        <>
          <LegendItem color="var(--t0)" shape="dot">
            Kill de {m.teams[0]}
          </LegendItem>
          <LegendItem color="var(--t1)" shape="dot">
            Kill de {m.teams[1]}
          </LegendItem>
          <LegendItem color="var(--muted)" shape="ring">
            Team kill
          </LegendItem>
          <LegendItem color="var(--text)" shape="tick">
            Pose du défuseur
          </LegendItem>
        </>
      }
    >
      {width > 0 && (
        <svg width={width} height={height} role="img" aria-label="Kills de chaque round selon le temps écoulé">
          {ticks.map((t) => (
            <g key={t}>
              <line x1={x(t)} x2={x(t)} y1={pad.top} y2={height - pad.bottom} className={t === 0 ? "axis" : "grid"} />
              <text x={x(t)} y={height - 8} className="tick" textAnchor="middle">
                {clock(t)}
              </text>
            </g>
          ))}
          {hasPrep && (
            <text x={pad.left + prepW / 2} y={height - 8} className="tick" textAnchor="middle">
              P
            </text>
          )}
          {m.rounds.map((r, i) => {
            const cy = pad.top + rowH * i + rowH / 2;
            const end = r.end.elapsedMs > 0 ? r.end.elapsedMs : Math.max(0, ...r.kills.map((k) => k.elapsedMs));
            return (
              <g key={r.number}>
                <text x={pad.left - 8} y={cy} className="tick" textAnchor="end" dominantBaseline="middle">
                  R{r.number}
                </text>
                <line x1={x(0)} x2={x(end)} y1={cy} y2={cy} className="lane" />
                {r.plant && (
                  <g
                    className="mark-group"
                    {...hover(setTip, x(r.plant.elapsedMs), cy, (
                      <>
                        <div className="tip-title">Round {r.number}</div>
                        <div>
                          <b>{clock(r.plant.elapsedMs)}</b> <span className="muted">pose{r.planter && ` par ${r.planter}`}</span>
                        </div>
                      </>
                    ))}
                  >
                    <rect x={x(r.plant.elapsedMs) - 6} y={cy - 11} width={12} height={22} fill="transparent" />
                    <rect x={x(r.plant.elapsedMs) - 1} y={cy - 9} width={2} height={18} rx={1} className="plant" />
                  </g>
                )}
                {r.kills.map((k, j) => {
                  const cx = x(k.elapsedMs);
                  const teamKill = k.killerTeam === k.victimTeam;
                  const color = k.killerTeam === 0 || k.killerTeam === 1 ? `var(--t${k.killerTeam})` : "var(--muted)";
                  return (
                    <g
                      key={j}
                      className="mark-group"
                      {...hover(setTip, cx, cy, (
                        <>
                          <div className="tip-title">
                            Round {r.number} · {clock(k.elapsedMs)}
                          </div>
                          <div>
                            <b>{k.killer || "?"}</b> <span className="muted">{k.headshot ? "headshot sur" : "tue"}</span> <b>{k.victim}</b>
                            {teamKill && <span className="neg"> (team kill)</span>}
                          </div>
                          {(k.killerRoom || k.victimRoom) && (
                            <div className="muted">
                              {k.killerRoom || "?"} → {k.victimRoom || "?"}
                            </div>
                          )}
                        </>
                      ))}
                    >
                      <circle cx={cx} cy={cy} r={11} fill="transparent" />
                      {teamKill ? (
                        <circle cx={cx} cy={cy} r={4} fill="none" stroke={color} strokeWidth={2} />
                      ) : (
                        <circle cx={cx} cy={cy} r={5} fill={color} className="kill-dot" />
                      )}
                    </g>
                  );
                })}
                <g>
                  <rect
                    x={width - pad.right + 10}
                    y={cy - 5}
                    width={10}
                    height={10}
                    rx={2}
                    fill={r.winner === 0 || r.winner === 1 ? `var(--t${r.winner})` : "var(--t-1)"}
                  />
                  <text x={width - pad.right + 24} y={cy} className="tick" dominantBaseline="middle">
                    {r.winner === 0 || r.winner === 1 ? m.teams[r.winner] : "?"}
                  </text>
                </g>
              </g>
            );
          })}
        </svg>
      )}
    </Frame>
  );
}

// ---------------------------------------------------------------------------
// Rooms

/** The rooms where most players die, deaths split by side. */
export function RoomBars({ rooms, limit = 12 }: { rooms: Room[]; limit?: number }) {
  const [ref, width] = useWidth<HTMLDivElement>();
  const [tip, setTip] = useState<Tip | null>(null);
  const total = (r: Room) => r.attack.deaths + r.defense.deaths;
  const rows = [...rooms].sort((a, b) => total(b) - total(a)).slice(0, limit);
  const rowH = 30;
  const barH = 18;
  const labelW = Math.min(200, width * 0.4);
  const pad = { top: 4, bottom: 4, right: 36 };
  const max = Math.max(1, ...rows.map(total));
  const len = (v: number) => (v / max) * (width - labelW - pad.right);
  const height = pad.top + rows.length * rowH + pad.bottom;

  return (
    <Frame
      frameRef={ref}
      tip={tip}
      label="Morts par pièce"
      legend={
        <>
          <LegendItem color="var(--attack)">Attaquants morts</LegendItem>
          <LegendItem color="var(--defense)">Défenseurs morts</LegendItem>
        </>
      }
    >
      {width > 0 && (
        <svg width={width} height={height} role="img" aria-label={`Les ${rows.length} pièces où l'on meurt le plus`}>
          {rows.map((r, i) => {
            const y = pad.top + i * rowH + (rowH - barH) / 2;
            const a = len(r.attack.deaths);
            const d = len(r.defense.deaths);
            const gap = a > 0 && d > 0 ? 2 : 0;
            return (
              <g
                key={r.room}
                className="mark-group"
                {...hover(setTip, labelW + a + d, y + barH / 2, (
                  <>
                    <div className="tip-title">{r.room}</div>
                    <TipRow color="var(--attack)" value={r.attack.deaths} label="attaquants morts" />
                    <TipRow color="var(--defense)" value={r.defense.deaths} label="défenseurs morts" />
                  </>
                ))}
              >
                <rect x={0} y={pad.top + i * rowH} width={width} height={rowH} fill="transparent" />
                <text x={labelW - 8} y={y + barH / 2} className="label" textAnchor="end" dominantBaseline="middle">
                  {truncate(r.room, Math.floor(labelW / 7))}
                </text>
                {/* The attack segment is square on both ends unless it is the whole bar. */}
                {a > 0 && (d > 0 ? <rect x={labelW} y={y} width={a} height={barH} fill="var(--attack)" /> : <path d={bar(labelW, y, barH, a, "right")} fill="var(--attack)" />)}
                {d > 0 && <path d={bar(labelW + a + gap, y, barH, d - gap, "right")} fill="var(--defense)" />}
                <text x={labelW + a + d + 6} y={y + barH / 2} className="value" dominantBaseline="middle">
                  {total(r)}
                </text>
              </g>
            );
          })}
        </svg>
      )}
    </Frame>
  );
}

function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, Math.max(1, n - 1)) + "…" : s;
}
