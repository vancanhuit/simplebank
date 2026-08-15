import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import AccountCard from "./AccountCard.svelte";
import type { Account } from "../api/types";

const account: Account = {
  id: "11111111-2222-3333-4444-555566667777",
  owner: "alice",
  balance: 48235,
  currency: "USD",
  created_at: "2026-01-15T10:00:00Z",
};

describe("AccountCard", () => {
  let originalClipboard: PropertyDescriptor | undefined;

  beforeEach(() => {
    vi.useFakeTimers();
    originalClipboard = Object.getOwnPropertyDescriptor(navigator, "clipboard");
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
    if (originalClipboard) {
      Object.defineProperty(navigator, "clipboard", originalClipboard);
    } else {
      delete (navigator as unknown as Record<string, unknown>).clipboard;
    }
  });

  it("renders the formatted balance and currency", () => {
    render(AccountCard, { props: { account } });

    expect(screen.getByText(/482\.35/)).toBeInTheDocument();
    expect(screen.getByText("USD")).toBeInTheDocument();
  });

  it("shows the full account number", () => {
    render(AccountCard, { props: { account } });

    expect(screen.getByText("11111111-2222-3333-4444-555566667777")).toBeInTheDocument();
  });

  it("exposes a copy-account-number action", () => {
    render(AccountCard, { props: { account } });

    expect(screen.getByRole("button", { name: /copy account number/i })).toBeInTheDocument();
  });

  it("exposes a send-money action", () => {
    render(AccountCard, { props: { account } });

    expect(screen.getByRole("button", { name: /send money/i })).toBeInTheDocument();
  });

  it("links to the account activity page", () => {
    render(AccountCard, { props: { account } });

    const link = screen.getByRole("link", { name: /activity/i });
    expect(link).toHaveAttribute("href", `/accounts/${account.id}`);
  });

  it("changes copy button name after successful copy without exposing icon", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      writable: true,
      configurable: true,
    });

    render(AccountCard, { props: { account } });

    const copyButton = screen.getByRole("button", { name: /copy account number/i });
    await fireEvent.click(copyButton);

    // Wait for async state update
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Account number copied" })).toBeInTheDocument();
    });

    expect(writeText).toHaveBeenCalledWith(account.id);

    // Assert SVG aria-hidden directly
    const checkIcon = screen
      .getByRole("button", { name: "Account number copied" })
      .querySelector("svg");
    expect(checkIcon).toHaveAttribute("aria-hidden", "true");
  });

  it("resets copy button name after 2 seconds", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      writable: true,
      configurable: true,
    });

    render(AccountCard, { props: { account } });

    const copyButton = screen.getByRole("button", { name: /copy account number/i });
    await fireEvent.click(copyButton);

    // Wait for async state update to Copied
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Account number copied" })).toBeInTheDocument();
    });

    // Advance timer by 2 seconds to trigger reset
    await vi.advanceTimersByTimeAsync(2000);

    expect(screen.getByRole("button", { name: /copy account number/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Account number copied" })).not.toBeInTheDocument();

    // Assert Copy icon has aria-hidden after reset
    const copyIcon = screen
      .getByRole("button", { name: /copy account number/i })
      .querySelector("svg");
    expect(copyIcon).toHaveAttribute("aria-hidden", "true");
  });

  it("clears pending timer on unmount before expiry", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      writable: true,
      configurable: true,
    });

    const { unmount } = render(AccountCard, { props: { account } });

    const copyButton = screen.getByRole("button", { name: /copy account number/i });
    await fireEvent.click(copyButton);

    // Wait for async state update to Copied
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Account number copied" })).toBeInTheDocument();
    });

    // Verify timer is pending
    expect(vi.getTimerCount()).toBeGreaterThan(0);

    unmount();

    // Verify timer was cleared without writing destroyed state
    expect(vi.getTimerCount()).toBe(0);
  });
});
