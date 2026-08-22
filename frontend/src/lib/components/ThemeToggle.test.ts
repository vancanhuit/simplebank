import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import { afterEach, describe, expect, it } from "vitest";
import { DARK_THEME, LIGHT_THEME, THEME_STORAGE_KEY } from "../theme";
import ThemeToggle from "./ThemeToggle.svelte";

describe("ThemeToggle", () => {
  afterEach(() => {
    cleanup();
    localStorage.clear();
    document.documentElement.dataset.theme = LIGHT_THEME;
  });

  it("describes and persists the theme it will switch to", async () => {
    document.documentElement.dataset.theme = LIGHT_THEME;
    render(ThemeToggle);

    await fireEvent.click(screen.getByRole("button", { name: "Switch to dark theme" }));

    expect(document.documentElement).toHaveAttribute("data-theme", DARK_THEME);
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe(DARK_THEME);
    expect(screen.getByRole("button", { name: "Switch to light theme" })).toBeInTheDocument();
  });
});
