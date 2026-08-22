import { cleanup, render, screen } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import DashboardPage from "./DashboardPage.svelte";
import { accounts } from "../stores/accounts.svelte";

describe("DashboardPage", () => {
  beforeEach(() => {
    accounts.loaded = true;
    accounts.items = [];
  });

  afterEach(() => {
    accounts.reset();
    cleanup();
  });

  it("offers one primary account-opening action when no accounts exist", () => {
    const { container } = render(DashboardPage);

    const accountLinks = screen
      .getAllByRole("link")
      .filter((link) => link.getAttribute("href") === "/accounts/new");

    expect(accountLinks).toHaveLength(1);
    expect(accountLinks[0]).toHaveAccessibleName("Open an account");
    expect(accountLinks[0]).toHaveClass("btn-primary");
    expect(container.querySelectorAll(".btn-primary")).toHaveLength(1);
  });

  it("keeps the hero actions when an account exists", () => {
    accounts.items = [
      {
        id: "11111111-2222-3333-4444-555566667777",
        owner: "alice",
        balance: 48235,
        currency: "USD",
        created_at: "2026-01-15T10:00:00Z",
      },
    ];

    render(DashboardPage);

    expect(screen.getByRole("link", { name: "Send money" })).toHaveAttribute("href", "/transfer");
    expect(screen.getByRole("link", { name: "New account" })).toHaveAttribute(
      "href",
      "/accounts/new",
    );
  });
});
