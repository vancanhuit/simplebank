import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { auth } from "../stores/auth.svelte";
import { router } from "../router.svelte";
import RegisterPage from "./RegisterPage.svelte";

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("RegisterPage", () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    history.replaceState({}, "", "/register");
    router.path = "/register";
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
    render(RegisterPage);
    await fireEvent.input(screen.getByRole("textbox", { name: "Username" }), {
      target: { value: "alice01" },
    });
    await fireEvent.input(screen.getByRole("textbox", { name: "Email" }), {
      target: { value: "alice@example.com" },
    });
    await fireEvent.input(screen.getByLabelText("Password"), {
      target: { value: "correct horse battery staple" },
    });

    await fireEvent.submit(screen.getByRole("button", { name: "Create account" }).closest("form")!);

    const fullName = screen.getByRole("textbox", { name: "Full name" });
    const fullNameError = screen.getByText("Enter your full name.");
    expect(fetchMock).not.toHaveBeenCalled();
    await waitFor(() => expect(fullName).toHaveFocus());
    expect(fullName).toHaveAttribute("aria-invalid", "true");
    expect(fullNameError).toHaveAttribute("role", "alert");
    expect(fullName.getAttribute("aria-describedby")?.split(" ")).toContain(fullNameError.id);

    await fireEvent.input(fullName, { target: { value: "Alice Smith" } });

    expect(fullName).not.toHaveAttribute("aria-invalid");
    expect(screen.queryByText("Enter your full name.")).not.toBeInTheDocument();
  });

  it("renders every empty-field error and focuses the first invalid field", async () => {
    render(RegisterPage);

    await fireEvent.submit(screen.getByRole("button", { name: "Create account" }).closest("form")!);

    const fullName = screen.getByRole("textbox", { name: "Full name" });
    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.getAllByRole("alert")).toHaveLength(4);
    expect(screen.getByText("Enter your full name.")).toBeInTheDocument();
    expect(screen.getByText("Enter your username.")).toBeInTheDocument();
    expect(screen.getByText("Enter a valid email address.")).toBeInTheDocument();
    expect(screen.getByText("Enter your password.")).toBeInTheDocument();
    await waitFor(() => expect(fullName).toHaveFocus());
  });

  it("submits normalized registration values without trimming the password", async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(202, { message: "check your email for verification instructions" }),
    );
    render(RegisterPage);

    await fireEvent.input(screen.getByRole("textbox", { name: "Full name" }), {
      target: { value: " Alice Smith " },
    });
    await fireEvent.input(screen.getByRole("textbox", { name: "Username" }), {
      target: { value: " alice01 " },
    });
    await fireEvent.input(screen.getByRole("textbox", { name: "Email" }), {
      target: { value: " alice@example.com " },
    });
    await fireEvent.input(screen.getByLabelText("Password"), {
      target: { value: " correct horse battery staple " },
    });
    await fireEvent.submit(screen.getByRole("button", { name: "Create account" }).closest("form")!);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledOnce());
    const [, options] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(JSON.parse(options.body as string)).toEqual({
      full_name: "Alice Smith",
      username: "alice01",
      email: "alice@example.com",
      password: " correct horse battery staple ",
    });
    await waitFor(() => expect(router.state).toEqual({ registered: true }));
  });
});
