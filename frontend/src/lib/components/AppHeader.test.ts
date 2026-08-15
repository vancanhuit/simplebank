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

  it("hides menu and close icons from the accessibility tree", async () => {
    auth.user = user;
    auth.accessToken = "access-token";
    render(AppHeader);

    const openButton = screen.getByRole("button", { name: "Open navigation" });
    const menuIcon = openButton.querySelector("svg");
    expect(menuIcon).toHaveAttribute("aria-hidden", "true");

    await fireEvent.click(openButton);

    const closeButton = screen.getByRole("button", { name: "Close navigation" });
    const closeIcon = closeButton.querySelector("svg");
    expect(closeIcon).toHaveAttribute("aria-hidden", "true");
  });

  it("reserves active marker width to prevent layout shift", () => {
    auth.user = user;
    auth.accessToken = "access-token";
    render(AppHeader);

    const desktopLinks = screen.getAllByRole("link", { name: "Overview" });
    const desktopLink = desktopLinks[0];

    // All nav links should have border-l-2 and border-transparent (marker width reserved)
    expect(desktopLink).toHaveClass("border-l-2");
    expect(desktopLink).toHaveClass("border-transparent");
  });

  it("asserts static fallback-class for active navigation marker on desktop and mobile links", async () => {
    auth.user = user;
    auth.accessToken = "access-token";
    render(AppHeader);

    // Open mobile menu to expose both desktop and mobile navigation links
    await fireEvent.click(screen.getByRole("button", { name: "Open navigation" }));

    const overviewLinks = screen.getAllByRole("link", { name: "Overview" });
    expect(overviewLinks).toHaveLength(2);
    const desktopLink = overviewLinks[0];
    const mobileLink = overviewLinks[1];

    // Verify desktop and mobile links are distinct elements
    expect(desktopLink).not.toBe(mobileLink);

    // Desktop link: assert marker width reservation and forced-colors fallback
    expect(desktopLink).toHaveClass("border-l-2");
    expect(desktopLink).toHaveClass("border-transparent");
    expect(desktopLink.className).toContain("forced-colors:aria-[current=page]:border-[Highlight]");

    // Mobile link: assert marker width reservation and forced-colors fallback
    expect(mobileLink).toHaveClass("border-l-2");
    expect(mobileLink).toHaveClass("border-transparent");
    expect(mobileLink.className).toContain("forced-colors:aria-[current=page]:border-[Highlight]");
  });
});
