// Types mirror the JSON written by the Go packages api, stats and rec.

import { useEffect, useState } from "react";
import { useWhere } from "./space";

/** How often something happened, and how often the round was won. */
export interface Rec {
  count: number;
  won: number;
}

/** A player's totals over a subset of rounds (stats.Line). */
export interface Line {
  rounds: number;
  won: number;
  kills: number;
  deaths: number;
  assists: number;
  headshots: number;
  kost: number;
  survived: number;
  openingKills: number;
  openingDeaths: number;
  plants: number;
  defuses: number;
}

export interface Player {
  id: string;
  name: string;
  team: string;
  rounds: number;
  kills: number;
  deaths: number;
  headshots: number;
  assists: number;
  openingKills: Rec;
  openingDeaths: Rec;
  tradeKills: number;
  tradedDeaths: number;
  /** Keyed by the number of enemies alive. */
  clutches: Record<string, Rec>;
  plants: Rec;
  defuses: number;
  kost: number;
  survived: number;
  /** By moment of the round: prep, then 30 s buckets, the last from 3:00 on. */
  killTiming: number[];
  deathTiming: number[];
  killRooms: Record<string, number>;
  deathRooms: Record<string, number>;
  bySide: Record<string, Line>;
  byMap: Record<string, Line>;
  byOperator: Record<string, Line>;
}

export interface TeamLine {
  rounds: Rec;
  openingKills: Rec;
  openingDeaths: Rec;
  deaths: number;
  tradedDeaths: number;
}

export interface Site {
  map: string;
  site: string;
  attack: Rec;
  defense: Rec;
  plants: Rec;
}

export interface Situation extends Rec {
  own: number;
  enemy: number;
}

export interface Team {
  name: string;
  rounds: Rec;
  openingKills: Rec;
  openingDeaths: Rec;
  deaths: number;
  tradedDeaths: number;
  sites: Site[] | null;
  bySide: Record<string, TeamLine>;
  byMap: Record<string, TeamLine>;
  /** Biggest advantage first. */
  situations: Situation[];
}

export interface RoomCount {
  kills: number;
  deaths: number;
}

export interface Room {
  map: string;
  room: string;
  attack: RoomCount;
  defense: RoomCount;
}

export interface Report {
  players: Player[];
  teams: Team[];
  rounds: number;
  skippedRounds: number;
  rooms: Room[] | null;
}

export interface Overview {
  /** The map the statistics are restricted to, "" for all. */
  map: string;
  /** The kind of match they are restricted to, "" for all. */
  kind: MatchKind | "";
  /** Every map played, whatever the filter. */
  maps: string[];
  matches: number;
  stats: Report;
  tradeWindowMs: number;
  /** The side each operator plays. */
  operatorSides: Record<string, "attack" | "defense">;
  /** Where each team name comes from. */
  teamNamedBy?: Record<string, TeamNamedBy>;
}

/**
 * Where a team name comes from: the replay (spectator recordings), the clan
 * tag of its players, a generic name and its most present player, or a
 * manager.
 */
export type TeamNamedBy = "replay" | "tag" | "player" | "manager";

/** What a match was played for, as the user labelled it. */
export type MatchKind = "pracc" | "official";

export const kindLabels: Record<MatchKind, string> = { pracc: "Pracc", official: "Officiel" };

export interface Settings {
  myTeam: string;
}

export interface MatchSummary {
  id: string;
  kind: MatchKind;
  map: string;
  teams: [string, string];
  score: [number, number];
  date: string;
  roundCount: number;
  superseded: number;
}

export type Phase = "prep" | "action" | "postplant";

export interface Clock {
  phase: Phase;
  remainingMs: number;
  /** Since the action phase started; negative during preparation. */
  elapsedMs: number;
}

export interface Kill extends Clock {
  killer: string;
  killerTeam: number;
  victim: string;
  victimTeam: number;
  headshot: boolean;
  killerRoom: string;
  victimRoom: string;
}

export type WinReason =
  | "attackers_eliminated"
  | "defenders_eliminated"
  | "defuser_detonated"
  | "defuser_disabled"
  | "time_expired"
  | "";

export interface RoundPlayer {
  name: string;
  team: number;
  side: "attack" | "defense" | "";
  operator: string;
  kills: number;
  deaths: number;
  assists: number;
  headshots: number;
}

export interface Round {
  number: number;
  score: [number, number];
  attacker: number;
  site: string;
  winner: number;
  winReason: WinReason;
  plant: Clock | null;
  planter: string;
  defuser: string;
  end: Clock;
  kills: Kill[];
  players: RoundPlayer[];
}

export interface MatchDetail extends MatchSummary {
  rounds: Round[];
  stats: Report;
}

type State<T> = { data?: T; error?: string; loading?: boolean };

// Answers are kept until a change the user makes (a match labelled, a team
// chosen) makes them stale.
const cache = new Map<string, unknown>();
let generation = 0;
const listeners = new Set<() => void>();

/** Drops every kept answer, so mounted pages fetch again. */
export function invalidate() {
  cache.clear();
  generation++;
  for (const l of listeners) l();
}

/**
 * The API path for path: relative to the current space, or global when it
 * starts with "/" (/me, /spaces).
 */
function resolve(base: string, path: string): string {
  return path.startsWith("/") ? path.slice(1) : base + path;
}

/** An answer that is not 2xx. */
export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

async function check(res: Response): Promise<Response> {
  if (!res.ok) throw new ApiError(res.status, `${res.status} ${(await res.text()).trim()}`);
  return res;
}

/**
 * Fetches a JSON endpoint of the API. While a new path loads, the previous
 * data is kept and loading is set, so the page does not flash.
 */
export function useApi<T>(path: string): State<T> & { status?: number } {
  const { base } = useWhere();
  const full = resolve(base, path);
  const [state, setState] = useState<State<T> & { status?: number }>(() =>
    cache.has(full) ? { data: cache.get(full) as T } : { loading: true },
  );
  const [gen, setGen] = useState(generation);
  useEffect(() => {
    const l = () => setGen(generation);
    listeners.add(l);
    return () => {
      listeners.delete(l);
    };
  }, []);
  useEffect(() => {
    if (cache.has(full)) {
      setState({ data: cache.get(full) as T });
      return;
    }
    let live = true;
    setState((s) => ({ data: s.data, loading: true }));
    fetch("/api/" + full)
      .then(check)
      .then((res) => res.json() as Promise<T>)
      .then(
        (data) => {
          cache.set(full, data);
          if (live) setState({ data });
        },
        (err: unknown) =>
          live && setState({ error: String(err), status: err instanceof ApiError ? err.status : undefined }),
      );
    return () => {
      live = false;
    };
  }, [full, gen]);
  return state;
}

/**
 * Sends a change to the API (path as for useApi), then refreshes every page.
 * Returns the decoded answer, if any.
 */
export function useSend() {
  const { base } = useWhere();
  return async <T = unknown>(method: "POST" | "PUT" | "DELETE", path: string, body?: unknown): Promise<T> => {
    const res = await fetch("/api/" + resolve(base, path), {
      method,
      headers: body === undefined ? undefined : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    }).then(check);
    invalidate();
    const text = await res.text();
    return (text ? JSON.parse(text) : undefined) as T;
  };
}
