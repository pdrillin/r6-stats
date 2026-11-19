import { useEffect, useState } from "react";
import { useWhere } from "./space";
import { useApi, useSend, type MatchSummary, type Settings, type Team } from "./api";

// The team the user plays in, kept by the server with the rest of the user's
// data. Earlier versions kept it in this browser: such a choice is sent to
// the server once, then forgotten here.
const LEGACY_KEY = "r6stats.team";

function takeLegacy(): string {
  try {
    const v = localStorage.getItem(LEGACY_KEY) ?? "";
    localStorage.removeItem(LEGACY_KEY);
    return v;
  } catch {
    return "";
  }
}

/**
 * The user's team, undefined until the server answered, and its setter. A
 * failed change is reported through the returned error.
 */
export function useMyTeam(): [string | undefined, (name: string) => void, string | undefined] {
  const settings = useApi<Settings>("settings");
  const send = useSend();
  const [error, setError] = useState<string>();
  const set = (name: string) => {
    setError(undefined);
    send("PUT", "settings", { myTeam: name }).catch((e: unknown) => setError(String(e)));
  };
  const team = settings.data?.myTeam;
  const { server } = useWhere();
  useEffect(() => {
    // Only r6rec serve ever kept the team in the browser.
    if (team === "" && !server) {
      const legacy = takeLegacy();
      if (legacy) set(legacy);
    }
    // Only the team matters: set always does the same thing.
  }, [team, server]);
  return [team, set, error ?? settings.error];
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
