import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/svelte";
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

  it("uses native daisyUI radios for currency choices", async () => {
    render(NewAccountPage);

    const euro = await screen.findByRole("radio", { name: /EUR/ });
    expect(euro).toHaveClass("radio", "radio-primary");
  });

  it("selects the first available currency when accounts are already loaded", async () => {
    render(NewAccountPage);

    // Wait for the opening limits to load and currency to be corrected.
    await waitFor(() => {
      expect(screen.getByRole("radio", { name: /EUR/ })).toBeChecked();
    });
  });

  it("gates creation on account inventory and initializes available currency after retry", async () => {
    accounts.loaded = false;
    accounts.items = [];
    let resolveInitialLoad!: () => void;
    const initialLoad = new Promise<void>((resolve) => {
      resolveInitialLoad = resolve;
    });
    const loadSpy = vi
      .spyOn(accounts, "load")
      .mockReturnValueOnce(initialLoad)
      .mockImplementationOnce(() => {
        accounts.error = null;
        accounts.items = [
          {
            id: "1",
            owner: "alice",
            currency: "USD",
            balance: 1000,
            created_at: "2026-01-01T00:00:00Z",
          },
        ];
        accounts.loaded = true;
        return Promise.resolve();
      });

    render(NewAccountPage);

    expect(await screen.findByRole("status")).toHaveTextContent("Loading your accounts");
    expect(screen.queryByRole("radio")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Create account" })).not.toBeInTheDocument();
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/api/v1/account-opening-limits", expect.any(Object));
    });

    accounts.error = "offline";
    resolveInitialLoad();

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Couldn't load your accounts. offline",
    );
    expect(screen.queryByRole("radio")).not.toBeInTheDocument();
    await fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByRole("radio", { name: /EUR/ })).toBeChecked();
    expect(screen.queryByRole("radio", { name: /USD/ })).not.toBeInTheDocument();
    expect(loadSpy).toHaveBeenCalledTimes(2);
  });

  it("keeps stale account-creation controls gated while a retry reload is pending", async () => {
    render(NewAccountPage);
    await waitFor(() => {
      expect(screen.getByRole("radio", { name: /EUR/ })).toBeEnabled();
    });

    accounts.error = "offline";
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Couldn't load your accounts. offline",
    );

    let resolveReload!: () => void;
    const reload = new Promise<void>((resolve) => {
      resolveReload = resolve;
    });
    vi.spyOn(accounts, "load").mockImplementationOnce(() => {
      accounts.loading = true;
      accounts.error = null;
      return reload.then(() => {
        accounts.items = [
          {
            id: "2",
            owner: "alice",
            currency: "EUR",
            balance: 2000,
            created_at: "2026-01-02T00:00:00Z",
          },
        ];
        accounts.loading = false;
      });
    });

    await fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByRole("status")).toHaveTextContent("Loading your accounts");
    expect(screen.queryByRole("radio")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Create account" })).not.toBeInTheDocument();

    resolveReload();

    expect(await screen.findByRole("radio", { name: /USD/ })).toBeChecked();
    expect(screen.queryByRole("radio", { name: /EUR/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create account" })).toBeEnabled();
  });

  it("offers only currencies the customer does not already hold", async () => {
    render(NewAccountPage);

    expect(await screen.findByRole("radio", { name: /EUR/ })).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: /VND/ })).toBeInTheDocument();
    expect(screen.queryByRole("radio", { name: /USD/ })).not.toBeInTheDocument();
  });

  it("applies opening policy to the deposit controls when the policy is ready", async () => {
    let resolveLimits!: (value: Response) => void;
    fetchMock.mockReturnValueOnce(
      new Promise<Response>((resolve) => {
        resolveLimits = resolve;
      }),
    );

    render(NewAccountPage);

    const euro = await screen.findByRole("radio", { name: /EUR/ });
    const deposit = screen.getByRole("spinbutton", { name: "Opening deposit (EUR)" });
    const submit = screen.getByRole("button", { name: "Create account" });
    expect(euro).toBeDisabled();
    expect(deposit).toBeDisabled();
    expect(submit).toBeDisabled();

    resolveLimits(jsonResponse(200, { EUR: 123456, VND: 500000 }));

    await waitFor(() => {
      expect(euro).toBeEnabled();
      expect(deposit).toBeEnabled();
      expect(submit).toBeEnabled();
    });
    expect(deposit).toHaveAttribute("step", "0.01");
    expect(deposit).toHaveAttribute("max", "1234.56");
    expect(deposit).toHaveAccessibleDescription(
      "Optional. Maximum €1,234.56. Leave blank to open at zero.",
    );

    await fireEvent.click(screen.getByRole("radio", { name: /VND/ }));

    const dongDeposit = screen.getByRole("spinbutton", { name: "Opening deposit (VND)" });
    expect(dongDeposit).toHaveAttribute("step", "1");
    expect(dongDeposit).toHaveAttribute("max", "500000");
    expect(dongDeposit).toHaveAccessibleDescription(
      "Optional. Maximum ₫500,000. Leave blank to open at zero.",
    );
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
