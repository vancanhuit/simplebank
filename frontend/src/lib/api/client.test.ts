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

  it("turns rate-limit metadata into an actionable message", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "rate limit exceeded" }), {
          status: 429,
          headers: { "Content-Type": "application/json", "Retry-After": "5" },
        }),
      ),
    );

    await expect(request("/users/login", { method: "POST", body: {} })).rejects.toMatchObject({
      status: 429,
      message: "Too many attempts. Try again in 5 seconds.",
    } satisfies Partial<ApiError>);
  });

  it("refreshes the token once and retries after a 401", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { error: "token has expired" }))
      .mockResolvedValueOnce(jsonResponse(200, { ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    auth.accessToken = "expired-token";
    const refresh = vi.spyOn(auth, "tryRefresh").mockImplementation(async () => {
      auth.accessToken = "refreshed-token";
      return true;
    });

    const data = await request<{ ok: boolean }>("/accounts", { authenticated: true });

    expect(data).toEqual({ ok: true });
    expect(refresh).toHaveBeenCalledOnce();
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[0][1]).toMatchObject({
      headers: { Authorization: "Bearer expired-token" },
    });
    expect(fetchMock.mock.calls[1][1]).toMatchObject({
      headers: { Authorization: "Bearer refreshed-token" },
    });
  });

  it("does not retry when the refresh fails", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(401, { error: "token has expired" }));
    vi.stubGlobal("fetch", fetchMock);
    vi.spyOn(auth, "tryRefresh").mockResolvedValue(false);

    await expect(request("/accounts", { authenticated: true })).rejects.toBeInstanceOf(ApiError);
    expect(fetchMock).toHaveBeenCalledOnce();
  });

  it("shares one refresh across concurrent 401 responses", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { error: "expired" }))
      .mockResolvedValueOnce(jsonResponse(401, { error: "expired" }))
      .mockResolvedValueOnce(jsonResponse(200, { id: "a" }))
      .mockResolvedValueOnce(jsonResponse(200, { id: "b" }));
    vi.stubGlobal("fetch", fetchMock);
    const refresh = vi.spyOn(auth, "tryRefresh").mockResolvedValue(true);

    const results = await Promise.all([
      request<{ id: string }>("/accounts/a", { authenticated: true }),
      request<{ id: string }>("/accounts/b", { authenticated: true }),
    ]);

    expect(results).toEqual([{ id: "a" }, { id: "b" }]);
    expect(refresh).toHaveBeenCalledOnce();
    expect(fetchMock).toHaveBeenCalledTimes(4);
  });

  it("does not retry when concurrent refresh fails", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { error: "expired" }))
      .mockResolvedValueOnce(jsonResponse(401, { error: "expired" }));
    vi.stubGlobal("fetch", fetchMock);
    const refresh = vi.spyOn(auth, "tryRefresh").mockResolvedValue(false);

    const results = await Promise.all([
      request<{ id: string }>("/accounts/a", { authenticated: true }).catch((e) => e),
      request<{ id: string }>("/accounts/b", { authenticated: true }).catch((e) => e),
    ]);

    expect(results).toEqual([expect.any(ApiError), expect.any(ApiError)]);
    expect(refresh).toHaveBeenCalledOnce();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("starts a new refresh for independent 401 cycles", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { error: "expired" }))
      .mockResolvedValueOnce(jsonResponse(200, { id: "a" }))
      .mockResolvedValueOnce(jsonResponse(401, { error: "expired" }))
      .mockResolvedValueOnce(jsonResponse(200, { id: "b" }));
    vi.stubGlobal("fetch", fetchMock);
    const refresh = vi.spyOn(auth, "tryRefresh").mockResolvedValue(true);

    const result1 = await request<{ id: string }>("/accounts/a", { authenticated: true });
    const result2 = await request<{ id: string }>("/accounts/b", { authenticated: true });

    expect(result1).toEqual({ id: "a" });
    expect(result2).toEqual({ id: "b" });
    expect(refresh).toHaveBeenCalledTimes(2);
    expect(fetchMock).toHaveBeenCalledTimes(4);
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
