import { useState } from "react";
import type { MatchSummary, Team } from "./api";

// The team the reader works with. Until spaces and accounts exist it is a
// choice kept in this browser only.
const KEY = "r6stats.team";

function read(): string {
  try {
    return localStorage.getItem(KEY) ?? "";
  } catch {
    return "";
  }
}

export function useMyTeam(): [string, (name: string) => void] {
  const [team, setTeam] = useState(read);
  const set = (name: string) => {
    setTeam(name);
    try {
      if (name) localStorage.setItem(KEY, name);
      else localStorage.removeItem(KEY);
    } catch {
      // Storage unavailable: the choice lasts until the page is closed.
    }
  };
  return [team, set];
}

/** A match from one team's point of view. */
export interface Result {
  match: MatchSummary;
  opponent: string;
  own: number;
  against: number;
  outcome: "win" | "loss" | "draw";
}

export function results(matches: MatchSummary[], team: string): Result[] {
  const out: Result[] = [];
  for (const m of matches) {
    const i = m.teams.indexOf(team);
    if (i < 0) continue;
    const own = m.score[i]!;
    const against = m.score[1 - i]!;
    out.push({
      match: m,
      opponent: m.teams[1 - i]!,
      own,
      against,
      outcome: own > against ? "win" : own < against ? "loss" : "draw",
    });
  }
  return out;
}

/** A bomb site seen from one side. */
export interface SiteSide {
  map: string;
  site: string;
  side: "attack" | "defense";
  count: number;
  won: number;
}

export function siteSides(t: Team): SiteSide[] {
  return (t.sites ?? []).flatMap((s) => [
    { map: s.map, site: s.site, side: "attack" as const, ...s.attack },
    { map: s.map, site: s.site, side: "defense" as const, ...s.defense },
  ]);
}

export function scoutPath(team: string): string {
  return "/teams/" + encodeURIComponent(team) + "/scout";
}
