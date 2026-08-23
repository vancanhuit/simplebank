import { afterEach, describe, expect, it, vi } from "vitest";
import { accounts } from "./accounts.svelte";

const staleAccount = {
  id: "11111111-1111-1111-1111-111111111111",
  owner: "alice",
  balance: 10_000,
  currency: "USD" as const,
  created_at: "2026-01-01T00:00:00Z",
};

const freshAccount = {
  id: "22222222-2222-2222-2222-222222222222",
  owner: "alice",
  balance: 20_000,
  currency: "USD" as const,
  created_at: "2026-01-02T00:00:00Z",
};

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function deferredFetches(): Array<(response: Response) => void> {
  const resolveFetches: Array<(response: Response) => void> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          resolveFetches.push(resolve);
        }),
    ),
  );
  return resolveFetches;
}

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

  it("keeps the newest account response when an older load finishes last", async () => {
    const resolveFetches = deferredFetches();

    const first = accounts.load();
    const second = accounts.load();
    resolveFetches[1](jsonResponse(200, [freshAccount]));
    await second;
    expect(accounts.items).toEqual([freshAccount]);

    resolveFetches[0](jsonResponse(200, [staleAccount]));
    await first;

    expect(accounts.items).toEqual([freshAccount]);
    expect(accounts.loading).toBe(false);
  });

  it("keeps loading while the newest load is still pending", async () => {
    const resolveFetches = deferredFetches();

    const first = accounts.load();
    const second = accounts.load();
    resolveFetches[0](jsonResponse(200, [staleAccount]));
    await first;

    expect(accounts.items).toEqual([]);
    expect(accounts.loaded).toBe(false);
    expect(accounts.loading).toBe(true);

    resolveFetches[1](jsonResponse(200, [freshAccount]));
    await second;

    expect(accounts.items).toEqual([freshAccount]);
    expect(accounts.loaded).toBe(true);
    expect(accounts.loading).toBe(false);
  });

  it("ignores an older load error after the newest load succeeds", async () => {
    const resolveFetches = deferredFetches();

    const first = accounts.load();
    const second = accounts.load();
    resolveFetches[1](jsonResponse(200, [freshAccount]));
    await second;

    resolveFetches[0](jsonResponse(500, { message: "stale failure" }));
    await first;

    expect(accounts.items).toEqual([freshAccount]);
    expect(accounts.error).toBeNull();
    expect(accounts.loading).toBe(false);
  });

  it("preserves loaded accounts when a refresh fails", async () => {
    let resolveRefresh!: (response: Response) => void;
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(200, [freshAccount]))
      .mockImplementationOnce(
        () =>
          new Promise<Response>((resolve) => {
            resolveRefresh = resolve;
          }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await expect(accounts.load()).resolves.toBe(true);
    const refreshing = accounts.load();
    expect(accounts.items).toEqual([freshAccount]);
    expect(accounts.loaded).toBe(true);

    resolveRefresh(jsonResponse(503, { error: "temporarily unavailable" }));
    await expect(refreshing).resolves.toBe(false);

    expect(accounts.items).toEqual([freshAccount]);
    expect(accounts.loaded).toBe(true);
    expect(accounts.error).toBe("temporarily unavailable");
  });

  it("treats an aborted load as cancellation without a user-visible error", async () => {
    const resolveFetches = deferredFetches();
    const controller = new AbortController();

    const load = accounts.load(controller.signal);
    controller.abort();
    resolveFetches[0](jsonResponse(200, [freshAccount]));

    await expect(load).resolves.toBe(false);
    expect(accounts.items).toEqual([]);
    expect(accounts.error).toBeNull();
    expect(accounts.loading).toBe(false);
  });
});
