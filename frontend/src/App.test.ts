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

const user = {
  username: "alice",
  full_name: "Alice Smith",
  email: "alice@example.com",
  is_email_verified: true,
  created_at: "2026-01-01T00:00:00Z",
};

function jsonResponse(status: number, body: unknown): Response {
  return new Response(status === 204 ? null : JSON.stringify(body), {
    status,
    headers: status === 204 ? {} : { "Content-Type": "application/json" },
  });
}

vi.mock("./lib/stores/accounts.svelte", () => ({ accounts: accountsMock }));

describe("App routing", () => {
  beforeEach(() => {
    history.replaceState({}, "", "/login");
    router.path = "/login";
    router.state = {};
    auth.clear();
    auth.initializing = false;
    vi.spyOn(auth, "init").mockResolvedValue();
    accountsMock.reset.mockClear();
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
    expect(history.length).toBe(historyLength + 1);
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
