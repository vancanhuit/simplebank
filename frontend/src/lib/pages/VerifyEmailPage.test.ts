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
  cleanup();
  history.replaceState({}, "", "/verify-email");
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("VerifyEmailPage", () => {
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

  it("does not offer a futile retry for an invalid link and directs the user to sign in", async () => {
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
