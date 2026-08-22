export const LIGHT_THEME = "simplebank-light";
export const DARK_THEME = "simplebank-dark";
export const THEME_STORAGE_KEY = "simplebank-theme";

export type ThemeName = typeof LIGHT_THEME | typeof DARK_THEME;

function isThemeName(value: string | null): value is ThemeName {
  return value === LIGHT_THEME || value === DARK_THEME;
}

export function resolveTheme(
  storage?: Pick<Storage, "getItem">,
  prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches,
): ThemeName {
  try {
    const saved = (storage ?? window.localStorage).getItem(THEME_STORAGE_KEY);
    if (isThemeName(saved)) return saved;
  } catch {
    // Storage can be unavailable in privacy-restricted contexts.
  }
  return prefersDark ? DARK_THEME : LIGHT_THEME;
}

export function applyTheme(
  theme: ThemeName,
  root: HTMLElement = document.documentElement,
): ThemeName {
  root.dataset.theme = theme;
  return theme;
}

export function saveTheme(theme: ThemeName, storage?: Pick<Storage, "setItem">): ThemeName {
  try {
    (storage ?? window.localStorage).setItem(THEME_STORAGE_KEY, theme);
  } catch {
    // The applied theme remains usable when persistence is unavailable.
  }
  return theme;
}

export function initializeTheme(
  storage?: Pick<Storage, "getItem">,
  prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches,
  root: HTMLElement = document.documentElement,
): ThemeName {
  return applyTheme(resolveTheme(storage, prefersDark), root);
}

export function toggleTheme(current: ThemeName): ThemeName {
  return current === LIGHT_THEME ? DARK_THEME : LIGHT_THEME;
}
