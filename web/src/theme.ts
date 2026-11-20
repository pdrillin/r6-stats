import { useEffect, useState } from "react";

/** The reader's choice: follow the system, or force a theme. */
export type ThemeChoice = "auto" | "light" | "dark";

export const themeLabels: Record<ThemeChoice, string> = { auto: "Auto", light: "Clair", dark: "Sombre" };

// Kept in this browser only: a display preference, not account data.
const KEY = "stratdata.theme";
const systemLight = window.matchMedia("(prefers-color-scheme: light)");

function stored(): ThemeChoice {
  try {
    const v = localStorage.getItem(KEY);
    if (v === "light" || v === "dark") return v;
  } catch {
    // Storage blocked: follow the system.
  }
  return "auto";
}

/** Sets data-theme on <html>, which style.css reads. */
function apply(choice: ThemeChoice) {
  const light = choice === "light" || (choice === "auto" && systemLight.matches);
  document.documentElement.dataset.theme = light ? "light" : "dark";
}

/**
 * Applies the stored theme before the first render, and follows the system
 * while the choice is "auto". Inline scripts are not allowed by the CSP, so
 * this runs from main.tsx.
 */
export function initTheme() {
  apply(stored());
  systemLight.addEventListener("change", () => apply(stored()));
}

const listeners = new Set<(c: ThemeChoice) => void>();

function setTheme(choice: ThemeChoice) {
  try {
    if (choice === "auto") localStorage.removeItem(KEY);
    else localStorage.setItem(KEY, choice);
  } catch {
    // The choice then lasts until the page is reloaded.
  }
  apply(choice);
  for (const l of listeners) l(choice);
}

/** The current choice and a setter, shared by every picker on the page. */
export function useTheme(): [ThemeChoice, (c: ThemeChoice) => void] {
  const [choice, setChoice] = useState(stored);
  useEffect(() => {
    listeners.add(setChoice);
    return () => {
      listeners.delete(setChoice);
    };
  }, []);
  return [choice, setTheme];
}
