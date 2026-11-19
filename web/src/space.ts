import { createContext, useContext } from "react";

/** A space as the server lists it for the signed-in user. */
export interface Space {
  id: string;
  name: string;
  kind: "team" | "league";
  createdAt: string;
  files: number;
  role: Role;
}

export type Role = "manager" | "player";

/**
 * Where the pages run. With r6server they run inside a space: API paths are
 * relative to it and the user's role decides what they may change. With
 * r6rec serve there is no space and no account: everything is allowed.
 */
export interface Where {
  /** Prefix of the space's API paths, "" without a space. */
  base: string;
  space?: Space;
  /** The user may change labels and settings. */
  canEdit: boolean;
  /** Accounts and spaces exist (r6server). */
  server: boolean;
  /** The signed-in user, with r6server. */
  me?: User;
}

/** A person signed in with Discord. */
export interface User {
  id: string;
  discordId: string;
  name: string;
  avatar: string;
}

/** The address a space's pages live under: they run with it as basename. */
export function spacePath(id: string, rest = ""): string {
  return `/s/${id}/${rest}`;
}

/** Signs out, then goes back to the start page. */
export async function signOut() {
  await fetch("/auth/logout", { method: "POST" });
  window.location.assign("/");
}

/** Sends the browser to Discord, coming back to the current page. */
export function signIn(next = window.location.pathname + window.location.search) {
  window.location.assign("/auth/login?next=" + encodeURIComponent(next));
}

export const LOCAL: Where = { base: "", canEdit: true, server: false };

export const WhereContext = createContext<Where>(LOCAL);

export function useWhere(): Where {
  return useContext(WhereContext);
}

export const roleLabels: Record<Role, string> = { manager: "Responsable", player: "Joueur" };
