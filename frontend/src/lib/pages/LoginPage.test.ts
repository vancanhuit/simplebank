import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { auth } from "../stores/auth.svelte";
import { router } from "../router.svelte";
import LoginPage from "./LoginPage.svelte";

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("LoginPage", () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    history.replaceState({}, "", "/login");
    router.path = "/login";
    router.state = {};
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    cleanup();
    auth.clear();
    vi.unstubAllGlobals();
  });

  it("blocks invalid submission, focuses the first invalid field, and clears its error on edit", async () => {
    render(LoginPage);
    await fireEvent.input(screen.getByLabelText("Password"), {
      target: { value: "password" },
    });

    await fireEvent.submit(screen.getByRole("button", { name: "Sign in" }).closest("form")!);

    const username = screen.getByRole("textbox", { name: "Username" });
    const usernameError = screen.getByText("Enter your username.");
    expect(fetchMock).not.toHaveBeenCalled();
    await waitFor(() => expect(username).toHaveFocus());
    expect(username).toHaveAttribute("aria-invalid", "true");
    expect(usernameError).toHaveAttribute("role", "alert");
    expect(usernameError).toHaveTextContent("Enter your username.");
    expect(username.getAttribute("aria-describedby")?.split(" ")).toContain(usernameError.id);

    await fireEvent.input(username, { target: { value: "alice01" } });

    expect(username).not.toHaveAttribute("aria-invalid");
    expect(screen.queryByText("Enter your username.")).not.toBeInTheDocument();
  });

  it("renders every empty-field error and focuses the first invalid field", async () => {
    render(LoginPage);

    await fireEvent.submit(screen.getByRole("button", { name: "Sign in" }).closest("form")!);

    const username = screen.getByRole("textbox", { name: "Username" });
    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.getAllByRole("alert")).toHaveLength(2);
    expect(screen.getByText("Enter your username.")).toBeInTheDocument();
    expect(screen.getByText("Enter your password.")).toBeInTheDocument();
    await waitFor(() => expect(username).toHaveFocus());
  });

  it("refocuses the first unchanged invalid field on repeat submission", async () => {
    render(LoginPage);
    await fireEvent.input(screen.getByLabelText("Password"), {
      target: { value: "password" },
    });
    const form = screen.getByRole("button", { name: "Sign in" }).closest("form")!;

    await fireEvent.submit(form);

    const username = screen.getByRole("textbox", { name: "Username" });
    await waitFor(() => expect(username).toHaveFocus());
    screen.getByRole("button", { name: "Sign in" }).focus();
    expect(username).not.toHaveFocus();

    await fireEvent.submit(form);

    await waitFor(() => expect(username).toHaveFocus());
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("submits a normalized username without trimming the password", async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(200, {
        access_token: "access-token",
        access_token_expires_at: "2026-08-22T23:00:00Z",
        session_id: "session-id",
        user: {
          username: "alice01",
          full_name: "Alice Smith",
          email: "alice@example.com",
          is_email_verified: true,
          created_at: "2026-08-22T22:00:00Z",
        },
      }),
    );
    render(LoginPage);

    await fireEvent.input(screen.getByRole("textbox", { name: "Username" }), {
      target: { value: " alice01 " },
    });
    await fireEvent.input(screen.getByLabelText("Password"), {
      target: { value: " password with spaces " },
    });
    await fireEvent.submit(screen.getByRole("button", { name: "Sign in" }).closest("form")!);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledOnce());
    const [, options] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(JSON.parse(options.body as string)).toEqual({
      username: "alice01",
      password: " password with spaces ",
    });
  });

  it("shows the session-expiry notice once and consumes its navigation state", async () => {
    const state = { returnTo: "/transfer", sessionExpired: true, preserved: "value" };
    history.replaceState(state, "", "/login");
    router.state = state;
    const first = render(LoginPage);

    expect(
      await screen.findByText("Your session expired. Sign in again to continue."),
    ).toBeInTheDocument();
    await waitFor(() => expect(history.state).toEqual({ preserved: "value" }));
    expect(router.state).toEqual({ preserved: "value" });

    first.unmount();
    render(LoginPage);

    expect(
      screen.queryByText("Your session expired. Sign in again to continue."),
    ).not.toBeInTheDocument();
  });

  it("navigates to a validated return path after successful login", async () => {
    const state = { returnTo: "/accounts/abc?tab=activity#latest" };
    history.replaceState(state, "", "/login");
    router.state = state;
    fetchMock.mockResolvedValue(
      jsonResponse(200, {
        access_token: "access-token",
        access_token_expires_at: "2026-08-24T12:00:00Z",
        session_id: "session-id",
        user: {
          username: "alice01",
          full_name: "Alice Smith",
          email: "alice@example.com",
          is_email_verified: true,
          created_at: "2026-08-22T22:00:00Z",
        },
      }),
    );
    render(LoginPage);

    await fireEvent.input(screen.getByRole("textbox", { name: "Username" }), {
      target: { value: "alice01" },
    });
    await fireEvent.input(screen.getByLabelText("Password"), {
      target: { value: "password" },
    });
    await fireEvent.submit(screen.getByRole("button", { name: "Sign in" }).closest("form")!);

    await waitFor(() => expect(router.path).toBe("/accounts/abc"));
    expect(`${location.pathname}${location.search}${location.hash}`).toBe(
      "/accounts/abc?tab=activity#latest",
    );
  });

  it("falls back to the dashboard after login when return state is malicious", async () => {
    const state = { returnTo: "https://evil.example/steal" };
    history.replaceState(state, "", "/login");
    router.state = state;
    fetchMock.mockResolvedValue(
      jsonResponse(200, {
        access_token: "access-token",
        access_token_expires_at: "2026-08-24T12:00:00Z",
        session_id: "session-id",
        user: {
          username: "alice01",
          full_name: "Alice Smith",
          email: "alice@example.com",
          is_email_verified: true,
          created_at: "2026-08-22T22:00:00Z",
        },
      }),
    );
    render(LoginPage);

    await fireEvent.input(screen.getByRole("textbox", { name: "Username" }), {
      target: { value: "alice01" },
    });
    await fireEvent.input(screen.getByLabelText("Password"), {
      target: { value: "password" },
    });
    await fireEvent.submit(screen.getByRole("button", { name: "Sign in" }).closest("form")!);

    await waitFor(() => expect(router.path).toBe("/"));
    expect(location.pathname).toBe("/");
    expect(location.origin).not.toBe("https://evil.example");
  });

  it("renders and falls back safely when return state is a malformed URL", async () => {
    const state = { returnTo: "/\\[::1" };
    history.replaceState(state, "", "/login");
    router.state = state;
    fetchMock.mockResolvedValue(
      jsonResponse(200, {
        access_token: "access-token",
        access_token_expires_at: "2026-08-24T12:00:00Z",
        session_id: "session-id",
        user: {
          username: "alice01",
          full_name: "Alice Smith",
          email: "alice@example.com",
          is_email_verified: true,
          created_at: "2026-08-22T22:00:00Z",
        },
      }),
    );

    render(LoginPage);

    expect(screen.getByRole("heading", { name: "Welcome back" })).toBeInTheDocument();
    await fireEvent.input(screen.getByRole("textbox", { name: "Username" }), {
      target: { value: "alice01" },
    });
    await fireEvent.input(screen.getByLabelText("Password"), {
      target: { value: "password" },
    });
    await fireEvent.submit(screen.getByRole("button", { name: "Sign in" }).closest("form")!);

    await waitFor(() => expect(router.path).toBe("/"));
    expect(location.pathname).toBe("/");
  });
});
