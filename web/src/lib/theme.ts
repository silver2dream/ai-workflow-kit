import { useSyncExternalStore } from "react";

export type Theme = "light" | "dark";

const KEY = "awk.workbench.theme";

// Light or dark is a convenience of this browser: kept by it, read with care,
// since a private window may refuse.
function stored(): Theme | null {
  try {
    const value = localStorage.getItem(KEY);
    return value === "light" || value === "dark" ? value : null;
  } catch {
    return null;
  }
}

function preferred(): Theme {
  return stored() ?? (window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light");
}

// One theme for the whole page: everything that shows it follows a change.
let current: Theme | null = null;
const listeners = new Set<() => void>();

function get(): Theme {
  if (current === null) {
    current = preferred();
    document.documentElement.classList.toggle("dark", current === "dark");
  }
  return current;
}

export function setTheme(theme: Theme) {
  current = theme;
  document.documentElement.classList.toggle("dark", theme === "dark");
  try {
    localStorage.setItem(KEY, theme);
  } catch {
    // Not kept: the next visit follows the system again.
  }
  listeners.forEach((listener) => listener());
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function useTheme(): [Theme, () => void] {
  const theme = useSyncExternalStore(subscribe, get, get);
  return [theme, () => setTheme(theme === "dark" ? "light" : "dark")];
}
