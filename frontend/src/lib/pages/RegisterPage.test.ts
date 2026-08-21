import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import { afterEach, describe, expect, it, vi } from "vitest";
import { auth } from "../stores/auth.svelte";
import RegisterPage from "./RegisterPage.svelte";

async function submitRegistration(password: string) {
  await fireEvent.input(screen.getByLabelText("Full name"), {
    target: { value: "Alice Example" },
  });
  await fireEvent.input(screen.getByLabelText("Username"), {
    target: { value: "alice" },
  });
  await fireEvent.input(screen.getByLabelText("Email"), {
    target: { value: "alice@example.com" },
  });
  await fireEvent.input(screen.getByLabelText("Password"), {
    target: { value: password },
  });
  await fireEvent.click(screen.getByRole("button", { name: "Create account" }));
}

describe("RegisterPage", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    cleanup();
  });

  it("describes byte policy without character-count attributes", () => {
    render(RegisterPage);

    const field = screen.getByLabelText("Password");

    expect(field).not.toHaveAttribute("minlength");
    expect(field).not.toHaveAttribute("maxlength");
    expect(screen.getByText("15 to 72 UTF-8 bytes.")).toBeInTheDocument();
  });

  it("accepts a multibyte password with 15 bytes and fewer than 15 characters", async () => {
    const password = "密".repeat(5);
    expect(password).toHaveLength(5);
    expect(new TextEncoder().encode(password)).toHaveLength(15);
    const register = vi.spyOn(auth, "register").mockResolvedValue({ message: "accepted" });
    render(RegisterPage);

    await submitRegistration(password);

    await waitFor(() => {
      expect(register).toHaveBeenCalledWith({
        full_name: "Alice Example",
        username: "alice",
        email: "alice@example.com",
        password,
      });
    });
  });

  it.each([
    ["fewer than 15 bytes", "密".repeat(4)],
    ["more than 72 bytes", "密".repeat(25)],
  ])("rejects a password with %s before the request", async (_case, password) => {
    const register = vi.spyOn(auth, "register").mockResolvedValue({ message: "accepted" });
    render(RegisterPage);

    await submitRegistration(password);

    expect(register).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent("Password must be 15 to 72 UTF-8 bytes.");
  });
});
