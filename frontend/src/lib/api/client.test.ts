import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { request, ApiError, toMessage } from "./client";
import { auth } from "../stores/auth.svelte";

function jsonResponse(status: number, body: unknown): Response {
  return new Response(body === undefined ? "" : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("request", () => {
  beforeEach(() => {
    auth.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("decodes a successful JSON response", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { id: "a1" }));
    vi.stubGlobal("fetch", fetchMock);

    const data = await request<{ id: string }>("/accounts/a1", { authenticated: true });

    expect(data).toEqual({ id: "a1" });
    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/accounts/a1");
  });

  it("throws ApiError with the server message on failure", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(jsonResponse(422, { error: "insufficient balance" })),
    );

    await expect(request("/transfers", { method: "POST", body: {} })).rejects.toMatchObject({
      status: 422,
      message: "insufficient balance",
    } satisfies Partial<ApiError>);
  });

  it("refreshes the token once and retries after a 401", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { error: "token has expired" }))
      .mockResolvedValueOnce(jsonResponse(200, { ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    const refresh = vi.spyOn(auth, "tryRefresh").mockResolvedValue(true);

    const data = await request<{ ok: boolean }>("/accounts", { authenticated: true });

    expect(data).toEqual({ ok: true });
    expect(refresh).toHaveBeenCalledOnce();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("does not retry when the refresh fails", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(401, { error: "token has expired" }));
    vi.stubGlobal("fetch", fetchMock);
    vi.spyOn(auth, "tryRefresh").mockResolvedValue(false);

    await expect(request("/accounts", { authenticated: true })).rejects.toBeInstanceOf(ApiError);
    expect(fetchMock).toHaveBeenCalledOnce();
  });
});

describe("toMessage", () => {
  it("uses the ApiError message", () => {
    expect(toMessage(new ApiError(404, "resource not found"))).toBe("resource not found");
  });

  it("falls back for unknown values", () => {
    expect(toMessage("boom")).toBe("Something went wrong. Please try again.");
  });
});
