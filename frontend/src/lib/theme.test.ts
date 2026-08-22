import { afterEach, describe, expect, it, vi } from "vitest";
import {
  DARK_THEME,
  LIGHT_THEME,
  THEME_STORAGE_KEY,
  applyTheme,
  initializeTheme,
  resolveTheme,
  saveTheme,
  toggleTheme,
} from "./theme";

describe("theme", () => {
  afterEach(() => {
    document.documentElement.removeAttribute("data-theme");
    vi.restoreAllMocks();
  });

  it("uses a valid saved theme before the system preference", () => {
    const storage = { getItem: vi.fn(() => LIGHT_THEME) };
    expect(resolveTheme(storage, true)).toBe(LIGHT_THEME);
    expect(storage.getItem).toHaveBeenCalledWith(THEME_STORAGE_KEY);
  });

  it("falls back to system preference for missing or invalid storage", () => {
    expect(resolveTheme({ getItem: () => null }, false)).toBe(LIGHT_THEME);
    expect(resolveTheme({ getItem: () => "unknown" }, true)).toBe(DARK_THEME);
  });

  it("falls back when storage access throws", () => {
    const storage = {
      getItem: () => {
        throw new DOMException("blocked");
      },
    };
    expect(resolveTheme(storage, true)).toBe(DARK_THEME);
  });

  it("applies, persists, and toggles supported themes", () => {
    const storage = { setItem: vi.fn() };
    expect(applyTheme(DARK_THEME)).toBe(DARK_THEME);
    expect(document.documentElement).toHaveAttribute("data-theme", DARK_THEME);
    expect(saveTheme(LIGHT_THEME, storage)).toBe(LIGHT_THEME);
    expect(storage.setItem).toHaveBeenCalledWith(THEME_STORAGE_KEY, LIGHT_THEME);
    expect(toggleTheme(LIGHT_THEME)).toBe(DARK_THEME);
  });

  it("initializes the resolved theme on the requested root", () => {
    const root = document.createElement("div");
    expect(initializeTheme({ getItem: () => DARK_THEME }, false, root)).toBe(DARK_THEME);
    expect(root).toHaveAttribute("data-theme", DARK_THEME);
  });

  it("does not fail when persistence is unavailable", () => {
    const storage = {
      setItem: () => {
        throw new DOMException("blocked");
      },
    };
    expect(saveTheme(DARK_THEME, storage)).toBe(DARK_THEME);
  });
});
