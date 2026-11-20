import { createSignal } from "solid-js";

export type ThemeMode = "system" | "light" | "dark";

const STORAGE_KEY = "debris.theme";

function readInitialTheme(): ThemeMode {
  const saved = typeof localStorage !== "undefined" ? localStorage.getItem(STORAGE_KEY) : null;
  if (saved === "system" || saved === "light" || saved === "dark") {
    return saved;
  }
  return "system";
}

function resolveDark(mode: ThemeMode): boolean {
  if (mode === "dark") return true;
  if (mode === "light") return false;
  return typeof window !== "undefined" && window.matchMedia("(prefers-color-scheme: dark)").matches;
}

function applyTheme(mode: ThemeMode) {
  if (typeof document === "undefined") return;
  const root = document.documentElement;
  const isDark = resolveDark(mode);
  root.classList.toggle("dark", isDark);
  root.style.colorScheme = isDark ? "dark" : "light";
}

const [theme, setThemeSignal] = createSignal<ThemeMode>(readInitialTheme());

if (typeof window !== "undefined") {
  applyTheme(theme());
  const mq = window.matchMedia("(prefers-color-scheme: dark)");
  mq.addEventListener("change", () => {
    if (theme() === "system") {
      applyTheme("system");
    }
  });
}

export function useTheme() {
  return theme;
}

export function setTheme(next: ThemeMode) {
  setThemeSignal(next);
  if (typeof localStorage !== "undefined") {
    localStorage.setItem(STORAGE_KEY, next);
  }
  applyTheme(next);
}

export function cycleTheme() {
  const current = theme();
  const next: ThemeMode = current === "system" ? "light" : current === "light" ? "dark" : "system";
  setTheme(next);
}
