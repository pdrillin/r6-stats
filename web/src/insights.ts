import type { Player, Rec, Team } from "./api";
import { percent, sideLabel } from "./format";
import { siteSides } from "./team";

/** A finding about a team, said in a sentence, with the sample behind it. */
export interface Insight {
  key: string;
  /** good: a strength, bad: something to work on, info: neither. */
  tone: "good" | "bad" | "info";
  text: string;
}

/** Below this many cases a rate says too little to be told as a finding. */
const MIN = 5;
/** Two rates are told apart only when this far from each other. */
const GAP = 0.15;
/** The most findings shown at once: the strongest come first. */
const MAX = 5;

const r = (x: Rec) => (x.count > 0 ? x.won / x.count : 0);
/** "65 % des rounds gagnés (15 sur 23)": the rate, what it counts, its sample. */
const said = (n: number, of: number, what: string) => `${percent(n, of)} ${what} (${n} sur ${of})`;

/**
 * What stands out for a team, strongest first. Every sentence carries its
 * sample, and nothing is said below MIN cases: with few matches the list is
 * short, which is honest.
 */
export function insights(t: Team, players: Player[]): Insight[] {
  const out: (Insight & { weight: number })[] = [];

  // Attack against defense.
  const att = t.bySide["attack"]?.rounds;
  const def = t.bySide["defense"]?.rounds;
  if (att && def && att.count >= MIN && def.count >= MIN && Math.abs(r(att) - r(def)) >= GAP) {
    const [strong, weak] = r(att) > r(def) ? ["attack", "defense"] : ["defense", "attack"];
    const s = strong === "attack" ? att : def;
    const w = weak === "attack" ? att : def;
    out.push({
      key: "sides",
      tone: "info",
      weight: Math.abs(r(att) - r(def)),
      text: `Plus solides en ${sideLabel(strong).toLowerCase()} : ${said(s.won, s.count, "des rounds gagnés")}, contre ${said(w.won, w.count, `en ${sideLabel(weak).toLowerCase()}`)}.`,
    });
  }

  // Best and worst map.
  const maps = Object.entries(t.byMap).filter(([, l]) => l.rounds.count >= MIN + 1);
  if (maps.length >= 2) {
    maps.sort((a, b) => r(b[1].rounds) - r(a[1].rounds));
    const [bestName, best] = maps[0]!;
    const [worstName, worst] = maps[maps.length - 1]!;
    if (r(best.rounds) - r(worst.rounds) >= GAP) {
      out.push({
        key: "best-map",
        tone: "good",
        weight: r(best.rounds) - 0.5,
        text: `Meilleure map : ${bestName}, ${said(best.rounds.won, best.rounds.count, "des rounds gagnés")}.`,
      });
      out.push({
        key: "worst-map",
        tone: "bad",
        weight: 0.5 - r(worst.rounds),
        text: `Map la plus difficile : ${worstName}, ${said(worst.rounds.won, worst.rounds.count, "des rounds gagnés")}.`,
      });
    }
  }

  // How often the team strikes first, and what it is worth.
  const rounds = t.rounds.count;
  const first = t.openingKills;
  if (rounds >= MIN * 2) {
    const share = first.count / rounds;
    if (share >= 0.55 || share <= 0.45) {
      out.push({
        key: "opening-share",
        tone: share >= 0.55 ? "good" : "bad",
        weight: Math.abs(share - 0.5),
        text: `Vous faites le premier kill dans ${said(first.count, rounds, "des rounds")}.`,
      });
    }
  }
  const lost = t.openingDeaths;
  if (first.count >= MIN && lost.count >= MIN) {
    out.push({
      key: "opening-worth",
      tone: "info",
      weight: r(first) - r(lost) - 0.3,
      text: `Le premier kill décide beaucoup : ${said(first.won, first.count, "des rounds gagnés quand vous l'obtenez")}, ${said(lost.won, lost.count, "quand vous perdez le premier joueur")}.`,
    });
  }

  // Strongest and weakest site, one side at a time.
  const sites = siteSides(t).filter((s) => s.count >= MIN - 1);
  for (const s of sites) {
    const rate = s.won / s.count;
    const where = `${s.site} (${s.map}) en ${sideLabel(s.side).toLowerCase()}`;
    if (rate >= 0.75) {
      out.push({ key: `site-${where}`, tone: "good", weight: rate - 0.5, text: `Point fort : ${where}, ${said(s.won, s.count, "des rounds gagnés")}.` });
    } else if (rate <= 0.25) {
      out.push({ key: `site-${where}`, tone: "bad", weight: 0.5 - rate, text: `À travailler : ${where}, ${said(s.won, s.count, "des rounds gagnés")}.` });
    }
  }

  // Deaths avenged by a teammate.
  if (t.deaths >= MIN * 2) {
    const traded = t.tradedDeaths / t.deaths;
    if (traded <= 0.15) {
      out.push({
        key: "trades",
        tone: "bad",
        weight: 0.3 - traded,
        text: `Peu de morts vengées dans les 5 secondes : ${said(t.tradedDeaths, t.deaths, "de vos morts")}.`,
      });
    } else if (traded >= 0.35) {
      out.push({
        key: "trades",
        tone: "good",
        weight: traded - 0.2,
        text: `Vos morts sont souvent vengées dans les 5 secondes : ${said(t.tradedDeaths, t.deaths, "de vos morts")}.`,
      });
    }
  }

  // The steadiest player.
  const regular = players.filter((p) => p.rounds >= MIN * 2).sort((a, b) => b.kost / b.rounds - a.kost / a.rounds)[0];
  if (regular) {
    out.push({
      key: "kost",
      tone: "info",
      weight: 0.05,
      text: `Le plus régulier : ${regular.name}, utile dans ${said(regular.kost, regular.rounds, "de ses rounds")} : kill, pose, survie ou mort vengée.`,
    });
  }

  return out.sort((a, b) => b.weight - a.weight).slice(0, MAX);
}
