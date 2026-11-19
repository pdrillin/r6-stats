import type { Rec, WinReason } from "./api";

/** Below this many cases a percentage is shown as unreliable. */
export const SMALL_SAMPLE = 10;

export function rate(n: number, of: number): number | null {
  return of > 0 ? n / of : null;
}

export function percent(n: number, of: number): string {
  return of > 0 ? `${Math.round((100 * n) / of)} %` : "–";
}

/**
 * A percentage always shown with its sample: "35 % 8/23". Small samples are
 * dimmed, since a 1v3 won once in two tries says little.
 */
export function Ratio({ n, of }: { n: number; of: number }) {
  if (of === 0) return <span className="muted">–</span>;
  const small = of < SMALL_SAMPLE;
  return (
    <span
      className={small ? "ratio small" : "ratio"}
      title={small ? `Échantillon faible : ${of} cas` : `${n} sur ${of}`}
    >
      {percent(n, of)} <span className="sample">{n}/{of}</span>
    </span>
  );
}

/** Rounds won out of rounds played for a Rec. */
export function RecRatio({ r }: { r: Rec }) {
  return <Ratio n={r.won} of={r.count} />;
}

/**
 * How many times something happened, then the share of those rounds won:
 * "13 · 77 % gagnés". The count is the sample of the percentage.
 */
export function RecCount({ r }: { r: Rec }) {
  if (r.count === 0) return <span className="muted">–</span>;
  return (
    <span className={r.count < SMALL_SAMPLE ? "ratio small-rate" : "ratio"} title={`${r.won} rounds gagnés sur ${r.count}`}>
      {r.count} <span className="sample">· {percent(r.won, r.count)} gagnés</span>
    </span>
  );
}

/** m:ss, with a minus sign during the preparation phase. */
export function clock(ms: number): string {
  const sign = ms < 0 ? "−" : "";
  const s = Math.floor(Math.abs(ms) / 1000);
  return `${sign}${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

export function date(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("fr-FR", { dateStyle: "medium", timeStyle: "short" });
}

export const winReasons: Record<WinReason, string> = {
  attackers_eliminated: "Attaquants éliminés",
  defenders_eliminated: "Défenseurs éliminés",
  defuser_detonated: "Défuseur explosé",
  defuser_disabled: "Défuseur désamorcé",
  time_expired: "Temps écoulé",
  "": "Inconnu",
};

export function sideLabel(side: string): string {
  switch (side) {
    case "attack":
      return "Attaque";
    case "defense":
      return "Défense";
  }
  return "Inconnu";
}

/** "+3", "0", "−2". */
export function diff(n: number): string {
  if (n > 0) return `+${n}`;
  if (n < 0) return `−${-n}`;
  return "0";
}
