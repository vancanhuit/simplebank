import { describe, it, expect, vi, afterEach } from "vitest";
import { AuthStore } from "./auth.svelte";
import { router } from "../router.svelte";
import { ApiError, toMessage } from "../api/client";

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

const validLoginResponse = {
  access_token: "new-access",
  access_token_expires_at: "2026-01-01T01:00:00Z",
  session_id: "session-1",
  user: verifiedUser,
};

const validRenewResponse = {
  access_token: "new-access",
  access_token_expires_at: "2026-01-01T01:00:00Z",
  user: verifiedUser,
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
    store.renewalUnavailable = true;
    store.sessionExpired = true;
    const generation = store.generation;

    store.clear();

    expect(store.generation).toBe(generation + 1);
    expect(store.renewalUnavailable).toBe(false);
    expect(store.sessionExpired).toBe(false);
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
    store.renewalUnavailable = true;
    store.sessionExpired = true;
    const generation = store.generation;

    const loginTask = store.login("alice", "password");

    expect(store.generation).toBe(generation + 1);
    expect(store.renewalUnavailable).toBe(false);
    expect(store.sessionExpired).toBe(false);
    resolveLogin(
      jsonResponse(200, {
        access_token: "access",
        access_token_expires_at: new Date(Date.now() + 60_000).toISOString(),
        session_id: "session-1",
        user: verifiedUser,
      }),
    );
    await loginTask;
  });

  it.each([
    ["an empty object", {}],
    ["wrong field types", { ...validLoginResponse, access_token: 42 }],
    ["an empty access token", { ...validLoginResponse, access_token: "" }],
    ["an invalid expiry", { ...validLoginResponse, access_token_expires_at: "later" }],
    ["an empty session id", { ...validLoginResponse, session_id: "" }],
    [
      "an unverified user",
      { ...validLoginResponse, user: { ...verifiedUser, is_email_verified: false } },
    ],
  ])("rejects login success containing %s before applying auth state", async (_name, body) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(200, body)));
    const store = new AuthStore();

    const error = await store.login("alice", "password").catch((reason: unknown) => reason);

    expect(error).toBeInstanceOf(ApiError);
    expect(error).toMatchObject({ kind: "invalid_response", status: 200, code: null });
    expect(toMessage(error)).toBe("SimpleBank returned an unexpected response. Please try again.");
    expect(store.user).toBeNull();
    expect(store.accessToken).toBeNull();
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

    expect(await store.tryRefresh()).toBe("refreshed");

    expect(store.accessToken).toBe("new-access");
    expect(store.user).toEqual(verifiedUser);
    expect(store.renewalUnavailable).toBe(false);
  });

  it.each([
    ["an empty object", {}],
    ["wrong field types", { ...validRenewResponse, user: { ...verifiedUser, full_name: 42 } }],
    ["an empty access token", { ...validRenewResponse, access_token: "" }],
    [
      "an invalid user date",
      { ...validRenewResponse, user: { ...verifiedUser, created_at: "today" } },
    ],
  ])("preserves auth when refresh success contains %s", async (_name, body) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(200, body)));
    const store = new AuthStore();
    store.user = verifiedUser;
    store.accessToken = "still-valid-access";
    const generation = store.generation;

    const outcome = await store.tryRefresh();

    expect(outcome).toBe("unavailable");
    expect(store.user).toEqual(verifiedUser);
    expect(store.accessToken).toBe("still-valid-access");
    expect(store.generation).toBe(generation);
    expect(store.renewalUnavailable).toBe(true);
  });

  it.each([
    ["null", null],
    ["false", false],
    ["zero", 0],
    ["an empty string", ""],
  ])("preserves auth when refresh success contains %s", async (_name, body) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(200, body)));
    const store = new AuthStore();
    store.user = verifiedUser;
    store.accessToken = "still-valid-access";
    const generation = store.generation;

    const outcome = await store.tryRefresh();

    expect(outcome).toBe("unavailable");
    expect(store.user).toEqual(verifiedUser);
    expect(store.accessToken).toBe("still-valid-access");
    expect(store.generation).toBe(generation);
    expect(store.renewalUnavailable).toBe(true);
    expect(store.sessionExpired).toBe(false);
  });

  it("shares the same generation-scoped refresh promise", async () => {
    let resolveRefresh!: (value: Response) => void;
    const fetchMock = vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          resolveRefresh = resolve;
        }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const store = new AuthStore();

    const automatic = store.tryRefresh();
    const manual = store.tryRefresh();

    expect(manual).toBe(automatic);
    expect(fetchMock).toHaveBeenCalledOnce();

    resolveRefresh(jsonResponse(200, validRenewResponse));
    await expect(automatic).resolves.toBe("refreshed");
    await expect(manual).resolves.toBe("refreshed");
  });

  it("treats an absent refresh cookie as signed out", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(204, undefined)));
    const store = new AuthStore();
    store.user = verifiedUser;
    store.accessToken = "stale-access";
    const generation = store.generation;

    const outcome = await store.tryRefresh();

    expect(outcome).toBe("no_session");
    expect(store.accessToken).toBeNull();
    expect(store.user).toBeNull();
    expect(store.generation).toBe(generation + 1);
    expect(store.renewalUnavailable).toBe(false);
    expect(store.sessionExpired).toBe(false);
  });

  it("treats an unverified refresh session as signed out", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse(200, {
          ...validRenewResponse,
          user: { ...verifiedUser, is_email_verified: false },
        }),
      ),
    );
    const store = new AuthStore();
    store.user = verifiedUser;
    store.accessToken = "stale-access";
    const generation = store.generation;

    const outcome = await store.tryRefresh();

    expect(outcome).toBe("no_session");
    expect(store.user).toBeNull();
    expect(store.accessToken).toBeNull();
    expect(store.generation).toBe(generation + 1);
    expect(store.renewalUnavailable).toBe(false);
    expect(store.sessionExpired).toBe(false);
  });

  it("treats a verification-required refresh error as signed out", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse(403, {
          code: "email_verification_required",
          error: "email verification required",
        }),
      ),
    );
    const store = new AuthStore();
    store.user = verifiedUser;
    store.accessToken = "stale-access";
    const generation = store.generation;

    const outcome = await store.tryRefresh();

    expect(outcome).toBe("no_session");
    expect(store.user).toBeNull();
    expect(store.accessToken).toBeNull();
    expect(store.generation).toBe(generation + 1);
    expect(store.renewalUnavailable).toBe(false);
    expect(store.sessionExpired).toBe(false);
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
    store.renewalUnavailable = true;
    store.sessionExpired = true;
    const generation = store.generation;

    const logoutTask = store.logout();

    expect(store.user).toBeNull();
    expect(store.accessToken).toBeNull();
    expect(store.generation).toBe(generation + 1);
    expect(store.renewalUnavailable).toBe(false);
    expect(store.sessionExpired).toBe(false);
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
            session_id: "session-1",
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

  it("retains local auth when refresh cannot reach the server", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("offline")));
    const store = new AuthStore();
    store.user = verifiedUser;
    store.accessToken = "stale-access";
    const generation = store.generation;

    const outcome = await store.tryRefresh();

    expect(outcome).toBe("unavailable");
    expect(store.user).toEqual(verifiedUser);
    expect(store.accessToken).toBe("stale-access");
    expect(store.generation).toBe(generation);
    expect(store.renewalUnavailable).toBe(true);
    expect(store.sessionExpired).toBe(false);
  });

  it("retains local auth when refresh receives a server error", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse(503, {
          code: "internal_error",
          error: "temporarily unavailable",
        }),
      ),
    );
    const store = new AuthStore();
    store.user = verifiedUser;
    store.accessToken = "stale-access";
    const generation = store.generation;

    const outcome = await store.tryRefresh();

    expect(outcome).toBe("unavailable");
    expect(store.user).toEqual(verifiedUser);
    expect(store.accessToken).toBe("stale-access");
    expect(store.generation).toBe(generation);
    expect(store.renewalUnavailable).toBe(true);
    expect(store.sessionExpired).toBe(false);
  });

  it("expires local auth when refresh is unauthorized", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse(401, {
          code: "invalid_refresh_token",
          error: "expired",
        }),
      ),
    );
    const store = new AuthStore();
    store.user = verifiedUser;
    store.accessToken = "stale-access";
    const generation = store.generation;

    const outcome = await store.tryRefresh();

    expect(outcome).toBe("expired");
    expect(store.user).toBeNull();
    expect(store.accessToken).toBeNull();
    expect(store.generation).toBe(generation + 1);
    expect(store.renewalUnavailable).toBe(false);
    expect(store.sessionExpired).toBe(true);
    expect(store.consumeSessionExpired()).toBe(true);
    expect(store.consumeSessionExpired()).toBe(false);
  });

  it("clears renewal unavailability after a successful retry", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockRejectedValueOnce(new Error("offline"))
        .mockResolvedValueOnce(
          jsonResponse(200, {
            access_token: "new-access",
            access_token_expires_at: new Date(Date.now() + 60_000).toISOString(),
            user: verifiedUser,
          }),
        ),
    );
    const store = new AuthStore();
    store.user = verifiedUser;
    store.accessToken = "stale-access";
    const generation = store.generation;

    expect(await store.tryRefresh()).toBe("unavailable");
    expect(store.renewalUnavailable).toBe(true);
    expect(await store.tryRefresh()).toBe("refreshed");

    expect(store.user).toEqual(verifiedUser);
    expect(store.accessToken).toBe("new-access");
    expect(store.generation).toBe(generation);
    expect(store.renewalUnavailable).toBe(false);
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
    expect(refreshResult).toBe("stale");
    expect(store.renewalUnavailable).toBe(false);
    expect(store.sessionExpired).toBe(false);
  });
});
