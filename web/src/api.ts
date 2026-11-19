// Types mirror the JSON written by the Go packages api, stats and rec.

import { useEffect, useState } from "react";

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
  matches: number;
  stats: Report;
  tradeWindowMs: number;
}

export interface MatchSummary {
  id: string;
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

type State<T> = { data?: T; error?: string };

/** Fetches a JSON endpoint of the API. */
export function useApi<T>(path: string): State<T> {
  const [state, setState] = useState<State<T>>({});
  useEffect(() => {
    let live = true;
    setState({});
    fetch("/api/" + path)
      .then(async (res) => {
        if (!res.ok) throw new Error(`${res.status} ${(await res.text()).trim()}`);
        return (await res.json()) as T;
      })
      .then(
        (data) => live && setState({ data }),
        (err: unknown) => live && setState({ error: String(err) }),
      );
    return () => {
      live = false;
    };
  }, [path]);
  return state;
}
