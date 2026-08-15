import { afterEach, describe, expect, it } from "vitest";
import { fireEvent, render, screen } from "@testing-library/svelte";
import { auth } from "../stores/auth.svelte";
import AppHeader from "./AppHeader.svelte";

const user = {
  username: "alexandria",
  full_name: "Alexandria Montgomery-Worthington Alexandria Montgomery-Worthington",
  email: "alexandria@example.com",
  is_email_verified: true,
  created_at: "2026-01-01T00:00:00Z",
};

describe("AppHeader", () => {
  afterEach(() => auth.clear());

  it("exposes primary destinations through a mobile disclosure", async () => {
    auth.user = user;
    auth.accessToken = "access-token";
    render(AppHeader);

    const trigger = screen.getByRole("button", { name: "Open navigation" });
    expect(trigger).toHaveAttribute("aria-expanded", "false");

    await fireEvent.click(trigger);

    expect(screen.getByRole("button", { name: "Close navigation" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
    expect(screen.getAllByRole("link", { name: "Overview" })).toHaveLength(2);
    expect(screen.getAllByRole("link", { name: "Transfer" })).toHaveLength(2);
  });

  it("closes the mobile navigation when selecting the current route", async () => {
    auth.user = user;
    auth.accessToken = "access-token";
    render(AppHeader);

    await fireEvent.click(screen.getByRole("button", { name: "Open navigation" }));
    await fireEvent.click(screen.getAllByRole("link", { name: "Overview" }).at(-1)!);

    const trigger = screen.getByRole("button", { name: "Open navigation" });
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(trigger).toHaveFocus();
    expect(screen.queryByRole("navigation", { name: "Mobile primary" })).not.toBeInTheDocument();
  });

  it("constrains long identities and keeps sign out on one line", () => {
    auth.user = user;
    auth.accessToken = "access-token";
    render(AppHeader);

    expect(screen.getByText(user.full_name)).toHaveClass("truncate");
    expect(screen.getByRole("link", { name: "SimpleBank" })).toHaveClass("min-h-11");
    expect(screen.getByRole("button", { name: "Sign out" })).toHaveClass("whitespace-nowrap");
  });
});
