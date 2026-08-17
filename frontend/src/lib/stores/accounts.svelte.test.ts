import { afterEach, describe, expect, it, vi } from "vitest";
import { accounts } from "./accounts.svelte";

const staleAccount = {
  id: "11111111-1111-1111-1111-111111111111",
  owner: "alice",
  balance: 10_000,
  currency: "USD" as const,
  created_at: "2026-01-01T00:00:00Z",
};

describe("AccountsStore", () => {
  afterEach(() => {
    accounts.reset();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("ignores a load response that arrives after reset", async () => {
    let resolveLoad: (response: Response) => void;
    vi.stubGlobal(
      "fetch",
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            resolveLoad = resolve;
          }),
      ),
    );

    const loadTask = accounts.load();
    accounts.reset();
    resolveLoad!(
      new Response(JSON.stringify([staleAccount]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    await loadTask;

    expect(accounts.items).toEqual([]);
    expect(accounts.loaded).toBe(false);
  });

  it("does not cache an account created before reset", async () => {
    let resolveCreate: (response: Response) => void;
    vi.stubGlobal(
      "fetch",
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            resolveCreate = resolve;
          }),
      ),
    );

    const createTask = accounts.create("USD", staleAccount.balance);
    accounts.reset();
    resolveCreate!(
      new Response(JSON.stringify(staleAccount), {
        status: 201,
        headers: { "Content-Type": "application/json" },
      }),
    );
    await createTask;

    expect(accounts.items).toEqual([]);
  });
});
