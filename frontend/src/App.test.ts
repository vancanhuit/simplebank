import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App.svelte";
import { navigate, router } from "./lib/router.svelte";
import { auth } from "./lib/stores/auth.svelte";

const accountsMock = vi.hoisted(() => ({
  items: [],
  loaded: true,
  loading: false,
  error: null,
  load: vi.fn(),
  reset: vi.fn(),
}));

const notificationsMock = vi.hoisted(() => ({
  items: [],
  recent: [],
  unreadCount: 0,
  loading: false,
  refreshing: false,
  error: null,
  loadingMore: false,
  loadMoreError: null,
  nextCursor: null,
  hasMore: false,
  toasts: [],
  start: vi.fn(),
  reset: vi.fn(),
  reconcile: vi.fn().mockResolvedValue(undefined),
  loadMore: vi.fn().mockResolvedValue(undefined),
  markRead: vi.fn().mockResolvedValue(undefined),
  markAllRead: vi.fn().mockResolvedValue(undefined),
  activityVersion: vi.fn().mockReturnValue(0),
}));

const user = {
  username: "alice",
  full_name: "Alice Smith",
  email: "alice@example.com",
  is_email_verified: true,
  created_at: "2026-01-01T00:00:00Z",
};

const renewResponse = {
  access_token: "renewed-access-token",
  access_token_expires_at: "2026-08-24T12:00:00Z",
  user,
};

function jsonResponse(status: number, body: unknown): Response {
  return new Response(status === 204 ? null : JSON.stringify(body), {
    status,
    headers: status === 204 ? {} : { "Content-Type": "application/json" },
  });
}

vi.mock("./lib/stores/accounts.svelte", () => ({ accounts: accountsMock }));
vi.mock("./lib/stores/notifications.svelte", () => ({ notifications: notificationsMock }));

describe("App routing", () => {
  beforeEach(() => {
    history.replaceState({}, "", "/login");
    router.path = "/login";
    router.state = {};
    auth.clear();
    auth.initializing = false;
    vi.spyOn(auth, "init").mockResolvedValue();
    accountsMock.load.mockReset();
    accountsMock.reset.mockClear();
    notificationsMock.start.mockClear();
    notificationsMock.reset.mockClear();
    notificationsMock.reconcile.mockClear();
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("updates title, announces navigation, and focuses main", async () => {
    render(App);
    await waitFor(() => expect(document.title).toBe("Sign in · SimpleBank"));

    navigate("/register");

    await waitFor(() => expect(document.title).toBe("Create account · SimpleBank"));
    await waitFor(() => {
      expect(document.querySelector('[aria-live="polite"]')).toHaveTextContent("Create account");
      expect(document.querySelector("main")).toHaveFocus();
    });
  });

  it("clears the account cache when auth resolves signed out", async () => {
    auth.initializing = true;
    render(App);
    expect(accountsMock.reset).not.toHaveBeenCalled();

    auth.initializing = false;

    await waitFor(() => expect(accountsMock.reset).toHaveBeenCalledOnce());
    expect(notificationsMock.reset).toHaveBeenCalledOnce();
  });

  it.each([
    ["a network failure", () => Promise.reject(new Error("offline"))],
    [
      "a server failure",
      () =>
        Promise.resolve(
          jsonResponse(503, { code: "internal_error", error: "temporarily unavailable" }),
        ),
    ],
  ])("offers startup session recovery after %s", async (_name, refresh) => {
    history.replaceState({}, "", "/transfer");
    router.path = "/transfer";
    auth.initializing = true;
    vi.restoreAllMocks();
    vi.stubGlobal("fetch", vi.fn(refresh));

    render(App);

    expect(await screen.findByText("We couldn't restore your session.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Welcome back" })).not.toBeInTheDocument();
    expect(auth.initializing).toBe(false);
    expect(accountsMock.reset).not.toHaveBeenCalled();
    expect(notificationsMock.reset).not.toHaveBeenCalled();
    expect(router.path).toBe("/transfer");
  });

  it("restores the originally requested protected page after startup retry succeeds", async () => {
    history.replaceState({}, "", "/transfer");
    router.path = "/transfer";
    auth.initializing = true;
    vi.restoreAllMocks();
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockRejectedValueOnce(new Error("offline"))
        .mockResolvedValueOnce(jsonResponse(200, renewResponse)),
    );
    render(App);

    await fireEvent.click(await screen.findByRole("button", { name: "Retry" }));

    expect(await screen.findByRole("heading", { name: "Send money" })).toBeInTheDocument();
    expect(router.path).toBe("/transfer");
    expect(accountsMock.load).toHaveBeenCalledOnce();
    expect(notificationsMock.reconcile).toHaveBeenCalledWith("manual");
  });

  it("announces and focuses protected content restored by startup retry", async () => {
    history.replaceState({}, "", "/transfer");
    router.path = "/transfer";
    auth.initializing = true;
    vi.restoreAllMocks();
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockRejectedValueOnce(new Error("offline"))
        .mockResolvedValueOnce(jsonResponse(200, renewResponse)),
    );
    render(App);
    const retry = await screen.findByRole("button", { name: "Retry" });
    retry.focus();
    expect(retry).toHaveFocus();

    await fireEvent.click(retry);

    expect(await screen.findByRole("heading", { name: "Send money" })).toBeInTheDocument();
    await waitFor(() => {
      expect(document.querySelector('[aria-live="polite"]')).toHaveTextContent("Send money");
      expect(document.querySelector("main")).toHaveFocus();
    });
  });

  it("keeps the protected page and session caches during transient authenticated renewal failure", async () => {
    history.replaceState({}, "", "/transfer");
    router.path = "/transfer";
    auth.user = user;
    auth.accessToken = "stale-access-token";
    auth.renewalUnavailable = true;

    render(App);

    expect(await screen.findByRole("heading", { name: "Send money" })).toBeInTheDocument();
    const alerts = screen.getAllByRole("alert");
    expect(alerts).toHaveLength(1);
    expect(alerts[0]).toHaveTextContent("We couldn't restore your session.");
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
    expect(accountsMock.reset).not.toHaveBeenCalled();
    expect(notificationsMock.reset).not.toHaveBeenCalled();
    expect(router.path).toBe("/transfer");
  });

  it("expires definitively with cache resets and a replacing return path", async () => {
    history.replaceState({}, "", "/transfer");
    router.path = "/transfer";
    auth.user = user;
    auth.accessToken = "stale-access-token";
    const historyLength = history.length;
    const replaceState = vi.spyOn(history, "replaceState");
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          jsonResponse(401, { code: "invalid_refresh_token", error: "Session expired" }),
        ),
    );
    render(App);

    await auth.tryRefresh();

    await waitFor(() => expect(router.path).toBe("/login"));
    expect(history.length).toBe(historyLength);
    expect(replaceState).toHaveBeenCalledWith(
      { returnTo: "/transfer", sessionExpired: true },
      "",
      "/login",
    );
    expect(accountsMock.reset).toHaveBeenCalled();
    expect(notificationsMock.reset).toHaveBeenCalled();
  });

  it("replaces a protected signed-out entry instead of pushing another entry", async () => {
    history.replaceState({}, "", "/transfer?source=shortcut");
    router.path = "/transfer";
    const historyLength = history.length;
    const replaceState = vi.spyOn(history, "replaceState");

    render(App);

    await waitFor(() => expect(router.path).toBe("/login"));
    expect(history.length).toBe(historyLength);
    expect(replaceState).toHaveBeenCalledWith(
      { returnTo: "/transfer?source=shortcut" },
      "",
      "/login",
    );
  });

  it("starts one notification session for an authenticated auth generation", async () => {
    history.replaceState({}, "", "/");
    router.path = "/";
    auth.user = user;
    auth.accessToken = "access-token";
    render(App);

    await waitFor(() => expect(notificationsMock.start).toHaveBeenCalledOnce());

    auth.accessToken = "refreshed-access-token";

    await new Promise((resolve) => setTimeout(resolve, 0));
    await waitFor(() => expect(notificationsMock.start).toHaveBeenCalledOnce());
  });

  it("protects the notifications route and publishes its title and announcement", async () => {
    history.replaceState({}, "", "/");
    router.path = "/";
    auth.user = user;
    auth.accessToken = "access-token";
    render(App);

    navigate("/notifications");

    expect(await screen.findByRole("heading", { name: "Notifications" })).toBeInTheDocument();
    await waitFor(() => expect(document.title).toBe("Notifications · SimpleBank"));
    expect(document.querySelector('[aria-live="polite"]')).toHaveTextContent("Notifications");

    auth.clear();

    expect(await screen.findByRole("heading", { name: "Welcome back" })).toBeInTheDocument();
    await waitFor(() => expect(router.path).toBe("/login"));
  });

  it("mounts persistent notification toasts only in authenticated chrome", async () => {
    const signedOut = render(App);
    expect(document.querySelector(".toast[aria-live='polite']")).not.toBeInTheDocument();
    signedOut.unmount();

    history.replaceState({}, "", "/");
    router.path = "/";
    auth.user = user;
    auth.accessToken = "access-token";
    render(App);

    expect(
      await waitFor(() => document.querySelector(".toast[aria-live='polite']")),
    ).toBeInTheDocument();
  });

  it("resets notification resources when the root unmounts", async () => {
    history.replaceState({}, "", "/");
    router.path = "/";
    auth.user = user;
    auth.accessToken = "access-token";
    const app = render(App);
    await waitFor(() => expect(notificationsMock.start).toHaveBeenCalledOnce());
    notificationsMock.reset.mockClear();

    app.unmount();

    expect(notificationsMock.reset).toHaveBeenCalledOnce();
  });

  it("shows one-shot server logout failure feedback after local sign-out", async () => {
    history.replaceState({}, "", "/");
    router.path = "/";
    auth.user = user;
    auth.accessToken = "access-token";
    const historyLength = history.length;
    let rejectLogout!: (reason?: unknown) => void;
    vi.stubGlobal(
      "fetch",
      vi.fn(
        () =>
          new Promise<Response>((_resolve, reject) => {
            rejectLogout = reject;
          }),
      ),
    );
    render(App);

    await fireEvent.click(screen.getByRole("button", { name: "Sign out" }));

    expect(await screen.findByRole("heading", { name: "Welcome back" })).toBeInTheDocument();
    expect(auth.user).toBeNull();
    expect(auth.accessToken).toBeNull();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();

    rejectLogout(new Error("offline"));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "You were signed out locally, but SimpleBank couldn't complete the server sign-out request.",
    );
    expect(accountsMock.reset).toHaveBeenCalled();
    const historyState: unknown = history.state;
    expect(
      historyState !== null && typeof historyState === "object" && "logoutFailed" in historyState,
    ).toBe(false);
    expect(history.length).toBe(historyLength);
  });

  it("keeps login rendered but disabled until logout settles, then permits login", async () => {
    history.replaceState({}, "", "/");
    router.path = "/";
    auth.user = user;
    auth.accessToken = "old-access-token";
    let resolveLogout!: (response: Response) => void;
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
            access_token: "new-access-token",
            access_token_expires_at: "2026-08-23T00:00:00Z",
            session_id: "new-session",
            user,
          }),
        );
      }
      return Promise.reject(new Error(`Unexpected URL: ${url}`));
    });
    vi.stubGlobal("fetch", fetchMock);
    render(App);

    await fireEvent.click(screen.getByRole("button", { name: "Sign out" }));

    expect(await screen.findByRole("heading", { name: "Welcome back" })).toBeInTheDocument();
    const username = screen.getByRole("textbox", { name: "Username" });
    const password = screen.getByLabelText("Password");
    const submit = screen.getByRole("button", { name: "Sign in" });
    expect(auth.loggingOut).toBe(true);
    expect(username).toBeDisabled();
    expect(password).toBeDisabled();
    expect(submit).toBeDisabled();
    await expect(auth.login("alice", "password")).rejects.toThrow(/sign-out/i);
    expect(fetchMock).toHaveBeenCalledOnce();
    const pendingHistoryLength = history.length;

    resolveLogout(jsonResponse(204, undefined));

    await waitFor(() => {
      expect(auth.loggingOut).toBe(false);
      expect(username).toBeEnabled();
      expect(password).toBeEnabled();
      expect(submit).toBeEnabled();
    });
    expect(history.length).toBe(pendingHistoryLength);
    await fireEvent.input(username, { target: { value: "alice" } });
    await fireEvent.input(password, { target: { value: "password" } });
    await fireEvent.click(submit);

    await waitFor(() => expect(auth.accessToken).toBe("new-access-token"));
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
