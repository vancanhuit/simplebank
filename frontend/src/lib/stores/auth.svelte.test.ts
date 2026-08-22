import { describe, it, expect, vi, afterEach } from "vitest";
import { AuthStore } from "./auth.svelte";

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

    const refreshed = await store.tryRefresh();

    expect(refreshed).toBe(false);
    expect(store.accessToken).toBeNull();
    expect(store.user).toBeNull();
  });

  it("clears local state only after server logout succeeds", async () => {
    let resolveLogout: (value: Response) => void;
    const logoutPromise = new Promise<Response>((resolve) => {
      resolveLogout = resolve;
    });

    const fetchMock = vi.fn((url: string | URL | Request) => {
      const urlString = typeof url === "string" ? url : url instanceof URL ? url.href : url.url;
      if (urlString.endsWith("/users/logout")) {
        return logoutPromise;
      }
      return Promise.reject(new Error(`Unexpected URL: ${urlString}`));
    });
    vi.stubGlobal("fetch", fetchMock);

    const store = new AuthStore();
    store.user = verifiedUser;
    store.accessToken = "access";

    // Call logout but don't resolve fetch yet.
    const logoutTask = store.logout();

    expect(store.user).toEqual(verifiedUser);
    expect(store.accessToken).toBe("access");

    // Now resolve server logout.
    resolveLogout!(jsonResponse(204, undefined));
    await logoutTask;

    // State must remain cleared.
    expect(store.user).toBeNull();
    expect(store.accessToken).toBeNull();
  });

  it("clears local state when server logout fails", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("offline")));
    const store = new AuthStore();
    store.user = verifiedUser;
    store.accessToken = "access";

    await expect(store.logout()).rejects.toThrow("offline");

    expect(store.user).toBeNull();
    expect(store.accessToken).toBeNull();
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
