import { cleanup, render, screen, waitFor } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import NewAccountPage from "./NewAccountPage.svelte";
import { accounts } from "../stores/accounts.svelte";

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("NewAccountPage", () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    // Clear previous call history to avoid false positives from prior tests.
    fetchMock.mockClear();

    // Seed one USD account so EUR becomes the first available currency.
    accounts.loaded = true;
    accounts.items = [
      {
        id: "1",
        owner: "alice",
        currency: "USD",
        balance: 1000,
        created_at: "2026-01-01T00:00:00Z",
      },
    ];

    fetchMock.mockResolvedValue(
      jsonResponse(200, {
        EUR: 100000,
        GBP: 100000,
        USD: 100000,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    accounts.reset();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    cleanup();
  });

  it("selects the first available currency when accounts are already loaded", async () => {
    render(NewAccountPage);

    // Wait for the opening limits to load and currency to be corrected.
    await waitFor(() => {
      expect(screen.getByRole("radio", { name: /EUR/ })).toBeChecked();
    });
  });

  it("fetches opening policy before account load completes and corrects currency after", async () => {
    accounts.loaded = false;
    let resolveAccounts!: () => void;
    const accountsPromise = new Promise<void>((resolve) => {
      resolveAccounts = resolve;
    });
    const loadSpy = vi.spyOn(accounts, "load").mockReturnValue(accountsPromise);

    render(NewAccountPage);

    // Opening-limits fetch starts immediately, before accounts.load() resolves.
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith("/api/v1/account-opening-limits", expect.any(Object)),
    );
    // Verify exact call count at this point (current test only, no stale history).
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(loadSpy).toHaveBeenCalled();

    // Resolve accounts with USD held.
    accounts.loaded = true;
    accounts.items = [
      {
        id: "1",
        owner: "alice",
        currency: "USD",
        balance: 1000,
        created_at: "2026-01-01T00:00:00Z",
      },
    ];
    resolveAccounts();

    // EUR becomes checked after accounts resolve.
    await waitFor(() => {
      expect(screen.getByRole("radio", { name: /EUR/ })).toBeChecked();
    });

    loadSpy.mockRestore();
  });

  it("exposes aria-busy on currency fieldset while policy loading is active", async () => {
    let resolveLimits!: (value: unknown) => void;
    const limitsPromise = new Promise((resolve) => {
      resolveLimits = resolve;
    });
    fetchMock.mockReturnValue(limitsPromise);

    render(NewAccountPage);

    const fieldset = screen.getByRole("group", { name: /currency/i });
    expect(fieldset).toHaveAttribute("aria-busy", "true");

    resolveLimits(
      jsonResponse(200, {
        EUR: 100000,
        GBP: 100000,
        USD: 100000,
      }),
    );

    await waitFor(() => {
      expect(fieldset).toHaveAttribute("aria-busy", "false");
    });
  });
});
