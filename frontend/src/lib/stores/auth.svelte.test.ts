import { describe, it, expect, vi, afterEach } from "vitest";
import { AuthStore } from "./auth.svelte";
import { router } from "../router.svelte";

function jsonResponse(status: number, body: unknown): Response {
  // 204 No Content must not have a body.
  const responseBody = status === 204 ? null : body === undefined ? "" : JSON.stringify(body);
  return new Response(responseBody, {
    status,
    headers: status === 204 ? {} : { "Content-Type": "application/json" },
  });
}

const verifiedUser = {
  username: "alice",
  full_name: "Alice Smith",
  email: "alice@example.com",
  is_email_verified: true,
  created_at: "2026-01-01T00:00:00Z",
};

describe("AuthStore", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    window.history.replaceState({}, "", "/");
    router.path = "/";
    router.state = {};
  });

  it("advances the auth generation when local auth is cleared", () => {
    const store = new AuthStore();
    const generation = store.generation;

    store.clear();

    expect(store.generation).toBe(generation + 1);
  });

  it("advances the auth generation when login starts", async () => {
    let resolveLogin!: (value: Response) => void;
    vi.stubGlobal(
      "fetch",
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            resolveLogin = resolve;
          }),
      ),
    );
    const store = new AuthStore();
    const generation = store.generation;

    const loginTask = store.login("alice", "password");

    expect(store.generation).toBe(generation + 1);
    resolveLogin(
      jsonResponse(200, {
        access_token: "access",
        access_token_expires_at: new Date(Date.now() + 60_000).toISOString(),
        user: verifiedUser,
      }),
    );
    await loginTask;
  });

  it("restores the access token from the refresh cookie", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse(200, {
          access_token: "new-access",
          access_token_expires_at: new Date(Date.now() + 60_000).toISOString(),
          user: verifiedUser,
        }),
      ),
    );
    const store = new AuthStore();

    await store.init();

    expect(store.accessToken).toBe("new-access");
    expect(store.user).toEqual(verifiedUser);
  });

  it("treats an absent refresh cookie as signed out", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(204, undefined)));
    const store = new AuthStore();
    store.user = verifiedUser;
    store.accessToken = "stale-access";
    const generation = store.generation;

    const refreshed = await store.tryRefresh();

    expect(refreshed).toBe(false);
    expect(store.accessToken).toBeNull();
    expect(store.user).toBeNull();
    expect(store.generation).toBe(generation + 1);
  });

  it("invalidates local auth before server logout settles", async () => {
    let resolveLogout!: (value: Response) => void;
    vi.stubGlobal(
      "fetch",
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            resolveLogout = resolve;
          }),
      ),
    );
    const store = new AuthStore();
    store.user = verifiedUser;
    store.accessToken = "access";
    const generation = store.generation;

    const logoutTask = store.logout();

    expect(store.user).toBeNull();
    expect(store.accessToken).toBeNull();
    expect(store.generation).toBe(generation + 1);
    resolveLogout(jsonResponse(204, undefined));
    await logoutTask;
    expect(router.state).toEqual({});
  });

  it("blocks login while logout is pending and allows it after logout completes", async () => {
    let resolveLogout!: (value: Response) => void;
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
      if (url.endsWith("/users/logout")) {
        return new Promise<Response>((resolve) => {
          resolveLogout = resolve;
        });
      }
      if (url.endsWith("/users/login")) {
        return Promise.resolve(
          jsonResponse(200, {
            access_token: "new-access",
            access_token_expires_at: new Date(Date.now() + 60_000).toISOString(),
            user: verifiedUser,
          }),
        );
      }
      return Promise.reject(new Error(`Unexpected URL: ${url}`));
    });
    vi.stubGlobal("fetch", fetchMock);
    const store = new AuthStore();
    store.user = verifiedUser;
    store.accessToken = "old-access";
    const generation = store.generation;

    const logoutTask = store.logout();

    expect(store.loggingOut).toBe(true);
    expect(store.generation).toBe(generation + 1);
    await expect(store.login("alice", "password")).rejects.toThrow(/sign-out/i);
    expect(fetchMock).toHaveBeenCalledOnce();
    expect(store.generation).toBe(generation + 1);

    resolveLogout(jsonResponse(204, undefined));
    await logoutTask;

    expect(store.loggingOut).toBe(false);
    await store.login("alice", "password");
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(store.generation).toBe(generation + 2);
    expect(store.accessToken).toBe("new-access");
  });

  it("replaces the current history entry when logout completes", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(204, undefined)));
    const store = new AuthStore();
    const historyLength = history.length;

    await store.logout();

    expect(location.pathname).toBe("/login");
    expect(history.length).toBe(historyLength);
    expect(router.path).toBe("/login");
    expect(router.state).toEqual({});
  });

  it("resolves with cleared local state and a one-shot notice when server logout fails", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("offline")));
    const store = new AuthStore();
    store.user = verifiedUser;
    store.accessToken = "access";
    const generation = store.generation;

    await expect(store.logout()).resolves.toBeUndefined();

    expect(store.user).toBeNull();
    expect(store.accessToken).toBeNull();
    expect(store.generation).toBe(generation + 1);
    expect(store.loggingOut).toBe(false);
    expect(router.state).toEqual({ logoutFailed: true });
    expect(history.state).toEqual({ logoutFailed: true });
  });

  it("advances the auth generation when refresh fails", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("offline")));
    const store = new AuthStore();
    store.user = verifiedUser;
    store.accessToken = "stale-access";
    const generation = store.generation;

    const refreshed = await store.tryRefresh();

    expect(refreshed).toBe(false);
    expect(store.user).toBeNull();
    expect(store.accessToken).toBeNull();
    expect(store.generation).toBe(generation + 1);
  });

  it("purges legacy refresh token on init", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse(200, {
          access_token: "new-access",
          access_token_expires_at: new Date(Date.now() + 60_000).toISOString(),
          user: verifiedUser,
        }),
      ),
    );

    const removeItem = vi.spyOn(Storage.prototype, "removeItem");
    const store = new AuthStore();

    await store.init();

    expect(removeItem).toHaveBeenCalledWith("simplebank.session");
    expect(store.accessToken).toBe("new-access");
    expect(store.user).toEqual(verifiedUser);
  });

  it("prevents late refresh from restoring auth after logout", async () => {
    let resolveRefresh: (value: Response) => void;
    const refreshPromise = new Promise<Response>((resolve) => {
      resolveRefresh = resolve;
    });
    let resolveLogout: (value: Response) => void;
    const logoutPromise = new Promise<Response>((resolve) => {
      resolveLogout = resolve;
    });

    const fetchMock = vi.fn((url: string | URL | Request) => {
      const urlString = typeof url === "string" ? url : url instanceof URL ? url.href : url.url;
      if (urlString.endsWith("/tokens/renew")) {
        return refreshPromise;
      }
      if (urlString.endsWith("/users/logout")) {
        return logoutPromise;
      }
      return Promise.reject(new Error(`Unexpected URL: ${urlString}`));
    });
    vi.stubGlobal("fetch", fetchMock);

    const store = new AuthStore();

    // Start refresh (but don't resolve yet).
    const refreshTask = store.tryRefresh();

    // Logout completes first.
    resolveLogout!(jsonResponse(204, undefined));
    await store.logout();

    expect(store.user).toBeNull();
    expect(store.accessToken).toBeNull();

    // Now the late refresh completes.
    resolveRefresh!(
      jsonResponse(200, {
        access_token: "stale-access",
        access_token_expires_at: new Date(Date.now() + 60_000).toISOString(),
        user: verifiedUser,
      }),
    );
    const refreshResult = await refreshTask;

    // Auth must remain cleared; the late refresh must not apply.
    expect(store.user).toBeNull();
    expect(store.accessToken).toBeNull();
    expect(refreshResult).toBe(false);
  });
});
