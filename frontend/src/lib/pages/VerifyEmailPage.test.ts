import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import { afterEach, describe, expect, it, vi } from "vitest";
import VerifyEmailPage from "./VerifyEmailPage.svelte";

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

afterEach(() => {
  vi.useRealTimers();
  cleanup();
  history.replaceState({}, "", "/verify-email");
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("VerifyEmailPage", () => {
  it.each(["unknown", "verified", "unverified"])(
    "shows identical public accepted guidance for %s email",
    async (kind) => {
      history.replaceState({}, "", "/verify-email");
      const fetchMock = vi.fn().mockResolvedValue(jsonResponse(202, { message: "accepted" }));
      vi.stubGlobal("fetch", fetchMock);
      render(VerifyEmailPage);
      const email = await screen.findByRole("textbox", { name: "Email" });
      await fireEvent.input(email, { target: { value: `${kind}@example.com` } });
      await fireEvent.click(screen.getByRole("button", { name: "Send verification email" }));
      expect(await screen.findByRole("status")).toHaveTextContent(
        "Request accepted. If this address needs verification, check your email for a new link. Delivery may take a few minutes.",
      );
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/v1/users/verify_email/resend",
        expect.objectContaining({
          body: JSON.stringify({ email: `${kind}@example.com` }),
          headers: { "Content-Type": "application/json" },
        }),
      );
      expect(location.search).toBe("");
    },
  );

  it("validates email, prevents duplicate pending requests and preserves email on failure", async () => {
    let resolve!: (value: Response) => void;
    const fetchMock = vi.fn(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    );
    vi.stubGlobal("fetch", fetchMock);
    render(VerifyEmailPage);
    const email = await screen.findByRole("textbox", { name: "Email" });
    const submit = screen.getByRole("button", { name: "Send verification email" });
    await fireEvent.click(submit);
    expect(email).toHaveAttribute("aria-invalid", "true");
    expect(email).toHaveFocus();
    expect(fetchMock).not.toHaveBeenCalled();
    await fireEvent.input(email, { target: { value: "alice@example.com" } });
    await fireEvent.click(submit);
    expect(submit).toBeDisabled();
    await fireEvent.submit(email.closest("form")!);
    expect(fetchMock).toHaveBeenCalledOnce();
    resolve(jsonResponse(503, { code: "internal_error" }));
    await screen.findByText("SimpleBank is temporarily unavailable. Please try again.");
    expect(email).toHaveValue("alice@example.com");
    expect(submit).toBeEnabled();
  });

  it("respects the supplied resend cooldown", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ code: "rate_limited" }), {
          status: 429,
          headers: { "Retry-After": "5" },
        }),
      ),
    );
    render(VerifyEmailPage);
    const email = await screen.findByRole("textbox", { name: "Email" });
    await fireEvent.input(email, { target: { value: "alice@example.com" } });
    vi.useFakeTimers();
    const submit = screen.getByRole("button", { name: "Send verification email" });
    await fireEvent.click(submit);
    await vi.advanceTimersByTimeAsync(0);
    expect(submit).toBeDisabled();
    await vi.advanceTimersByTimeAsync(4_999);
    expect(submit).toBeDisabled();
    await vi.advanceTimersByTimeAsync(1);
    expect(submit).toBeEnabled();
    expect(email).toHaveValue("alice@example.com");
  });
  it("retries the captured verification request after removing credentials from the URL", async () => {
    history.replaceState({}, "", "/verify-email?id=user%2F1&code=secret%20code");
    const fetchMock = vi
      .fn()
      .mockRejectedValueOnce(new TypeError("private network detail"))
      .mockResolvedValueOnce(jsonResponse(200, { is_verified: true }));
    vi.stubGlobal("fetch", fetchMock);

    render(VerifyEmailPage);

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(
      "We couldn't reach SimpleBank. Check your connection and try again.",
    );
    expect(window.location.search).toBe("");

    await fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByText("Email verified")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/v1/users/verify_email?id=user%2F1&code=secret%20code",
      expect.any(Object),
    );
  });

  it("offers public resend rather than a futile verification retry for an invalid link", async () => {
    history.replaceState({}, "", "/verify-email?id=user&code=expired");
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(jsonResponse(400, { code: "invalid_verification_link" })),
    );

    render(VerifyEmailPage);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "This verification link is invalid or has expired.",
    );
    expect(screen.queryByRole("button", { name: "Retry" })).not.toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Email" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Send verification email" })).toBeEnabled();
    expect(screen.getByRole("link", { name: "Continue to sign in" })).toHaveAttribute(
      "href",
      "/login",
    );
  });

  it("does not send incomplete verification links", async () => {
    history.replaceState({}, "", "/verify-email?id=user");
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    render(VerifyEmailPage);

    await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());
    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: "Retry" })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Continue to sign in" })).toHaveAttribute(
      "href",
      "/login",
    );
  });
});
