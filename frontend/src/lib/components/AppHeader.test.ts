import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import { router } from "../router.svelte";
import { auth } from "../stores/auth.svelte";
import { accounts } from "../stores/accounts.svelte";
import AppHeader from "./AppHeader.svelte";

const user = {
  username: "alexandria",
  full_name: "Alexandria Montgomery-Worthington Alexandria Montgomery-Worthington",
  email: "alexandria@example.com",
  is_email_verified: true,
  created_at: "2026-01-01T00:00:00Z",
};

describe("AppHeader", () => {
  afterEach(() => {
    auth.clear();
    router.path = "/";
    router.state = {};
    vi.unstubAllGlobals();
  });

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

  it("closes the mobile navigation on Escape and restores trigger focus", async () => {
    auth.user = user;
    auth.accessToken = "access-token";
    render(AppHeader);

    await fireEvent.click(screen.getByRole("button", { name: "Open navigation" }));
    await fireEvent.keyDown(window, { key: "Escape" });

    const trigger = screen.getByRole("button", { name: "Open navigation" });
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(trigger).toHaveFocus();
    expect(screen.queryByRole("navigation", { name: "Mobile primary" })).not.toBeInTheDocument();
  });

  it("closes the mobile navigation when the route changes", async () => {
    auth.user = user;
    auth.accessToken = "access-token";
    router.path = "/";
    render(AppHeader);

    await fireEvent.click(screen.getByRole("button", { name: "Open navigation" }));
    expect(screen.getByRole("navigation", { name: "Mobile primary" })).toBeInTheDocument();

    router.path = "/transfer";

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Open navigation" })).toHaveAttribute(
        "aria-expanded",
        "false",
      );
      expect(screen.queryByRole("navigation", { name: "Mobile primary" })).not.toBeInTheDocument();
    });
  });

  it("constrains long identities and keeps sign out on one line", () => {
    auth.user = user;
    auth.accessToken = "access-token";
    render(AppHeader);

    expect(screen.getByText(user.full_name)).toHaveClass("truncate");
    expect(screen.getByRole("link", { name: "SimpleBank" })).toHaveClass("min-h-11");
    expect(screen.getByRole("button", { name: "Sign out" })).toHaveClass("whitespace-nowrap");
  });

  it("clears session-scoped state without retaining failed logout feedback", async () => {
    auth.user = user;
    auth.accessToken = "access-token";
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("offline")));
    const reset = vi.spyOn(accounts, "reset");
    render(AppHeader);

    await fireEvent.click(screen.getByRole("button", { name: "Sign out" }));

    await waitFor(() => {
      expect(reset).toHaveBeenCalledOnce();
      expect(auth.user).toBeNull();
      expect(auth.accessToken).toBeNull();
    });
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
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

  it("uses daisyUI navigation and exposes theme switching", () => {
    auth.user = user;
    auth.accessToken = "access-token";
    router.path = "/";
    render(AppHeader);

    expect(screen.getByRole("banner").querySelector(".navbar")).toBeInTheDocument();
    expect(
      screen.getByRole("navigation", { name: "Primary" }).querySelector(".menu"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /switch to (dark|light) theme/i }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Overview" })).toHaveAttribute("aria-current", "page");
  });

  it("keeps desktop and mobile navigation links at least 44px tall", async () => {
    auth.user = user;
    auth.accessToken = "access-token";
    render(AppHeader);

    await fireEvent.click(screen.getByRole("button", { name: "Open navigation" }));

    for (const link of screen.getAllByRole("link", { name: /^(Overview|Transfer)$/ })) {
      expect(link).toHaveClass("min-h-11");
    }
  });

  it("keeps a reserved forced-colors marker on desktop and mobile current links", async () => {
    auth.user = user;
    auth.accessToken = "access-token";
    router.path = "/";
    render(AppHeader);

    await fireEvent.click(screen.getByRole("button", { name: "Open navigation" }));

    for (const link of screen.getAllByRole("link", { name: "Overview" })) {
      expect(link).toHaveAttribute("aria-current", "page");
      expect(link).toHaveClass("border-2", "border-transparent");
      expect(link.className).toContain("forced-colors:aria-[current=page]:border-[Highlight]");
    }
  });
});
