import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { request, requestResponse, ApiError, isRetryable, toMessage } from "./client";
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

  it("classifies API failures by status and code", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse(422, {
          code: "insufficient_balance",
          error: "database password leaked",
        }),
      ),
    );

    await expect(request("/transfers", { method: "POST", body: {} })).rejects.toMatchObject({
      kind: "api",
      status: 422,
      code: "insufficient_balance",
    } satisfies Partial<ApiError>);
  });

  it("redacts server text when formatting an API failure", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse(422, {
          code: "insufficient_balance",
          error: "database password leaked",
        }),
      ),
    );

    const error = await request("/transfers", { method: "POST", body: {} }).catch(
      (reason: unknown) => reason,
    );

    expect(toMessage(error)).toBe("You don't have enough money in this account.");
  });

  it("formats unknown API codes by status instead of server text", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          jsonResponse(503, { code: "internal_detail", error: "database password leaked" }),
        ),
    );

    const error = await request("/accounts").catch((reason: unknown) => reason);

    expect(toMessage(error)).toBe("SimpleBank is temporarily unavailable. Please try again.");
  });

  it("classifies fetch rejections without exposing native messages", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("Failed to fetch secret host")));

    const error = await request("/accounts").catch((reason: unknown) => reason);

    expect(error).toMatchObject({ kind: "network", status: null, code: null });
    expect(toMessage(error)).toBe(
      "We couldn't reach SimpleBank. Check your connection and try again.",
    );
  });

  it("keeps classified aborts silent and non-retryable", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockRejectedValue(new DOMException("private abort detail", "AbortError")),
    );

    const error = await request("/accounts").catch((reason: unknown) => reason);

    expect(error).toMatchObject({ kind: "aborted", status: null, code: null });
    expect(toMessage(error)).toBe("");
    expect(toMessage(error)).not.toContain("private abort detail");
    expect(isRetryable(error)).toBe(false);
  });

  it("classifies malformed successful JSON without exposing parser text", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("not json", { status: 200 })));

    const error = await request("/accounts").catch((reason: unknown) => reason);

    expect(error).toMatchObject({ kind: "invalid_response", status: 200, code: null });
    expect(toMessage(error)).toBe("SimpleBank returned an unexpected response. Please try again.");
  });

  it("retains a non-retryable 400 status when its response body is unreadable", async () => {
    const response = jsonResponse(400, { code: "invalid_request", error: "private server text" });
    vi.spyOn(response, "text").mockRejectedValue(new Error("private body reader text"));
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response));

    const error = await request("/accounts").catch((reason: unknown) => reason);

    expect(error).toMatchObject({ kind: "api", status: 400, code: null });
    expect(toMessage(error)).toBe(
      "We couldn't complete your request. Please check your details and try again.",
    );
    expect(toMessage(error)).not.toContain("private body reader text");
    expect(isRetryable(error)).toBe(false);
  });

  it("retains a retryable 503 status when its response body is unreadable", async () => {
    const response = jsonResponse(503, { code: "internal_error", error: "private server text" });
    vi.spyOn(response, "text").mockRejectedValue(new Error("private body reader text"));
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response));

    const error = await request("/accounts").catch((reason: unknown) => reason);

    expect(error).toMatchObject({ kind: "api", status: 503, code: null });
    expect(toMessage(error)).toBe("SimpleBank is temporarily unavailable. Please try again.");
    expect(toMessage(error)).not.toContain("private body reader text");
    expect(isRetryable(error)).toBe(true);
  });

  it("preserves no-content success as undefined", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(null, { status: 204 })));

    await expect(request("/users/logout", { method: "POST" })).resolves.toBeUndefined();
  });

  it("stores rate-limit metadata and formats an actionable message", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ code: "rate_limit_exceeded", error: "private detail" }), {
          status: 429,
          headers: { "Content-Type": "application/json", "Retry-After": "5" },
        }),
      ),
    );

    const error = await request("/users/login", { method: "POST", body: {} }).catch(
      (reason: unknown) => reason,
    );

    expect(error).toMatchObject({
      kind: "api",
      status: 429,
      retryAfterSeconds: 5,
    } satisfies Partial<ApiError>);
    expect(toMessage(error)).toBe("Too many attempts. Try again in 5 seconds.");
  });

  it("refreshes the token once and retries after a 401", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { code: "token_expired", error: "expired" }))
      .mockResolvedValueOnce(jsonResponse(200, { ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    auth.accessToken = "expired-token";
    const refresh = vi.spyOn(auth, "tryRefresh").mockImplementation(() => {
      auth.accessToken = "refreshed-token";
      return Promise.resolve(true);
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
    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonResponse(401, { code: "token_expired", error: "expired" }));
    vi.stubGlobal("fetch", fetchMock);
    vi.spyOn(auth, "tryRefresh").mockResolvedValue(false);

    await expect(request("/accounts", { authenticated: true })).rejects.toBeInstanceOf(ApiError);
    expect(fetchMock).toHaveBeenCalledOnce();
  });

  it("does not retry a 401 after the auth generation changes", async () => {
    let resolveFirst!: (response: Response) => void;
    const fetchMock = vi
      .fn()
      .mockImplementationOnce(
        () =>
          new Promise<Response>((resolve) => {
            resolveFirst = resolve;
          }),
      )
      .mockResolvedValueOnce(jsonResponse(200, { ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    auth.accessToken = "old-token";
    const refresh = vi.spyOn(auth, "tryRefresh").mockResolvedValue(true);

    const pending = request("/transfers", { method: "POST", authenticated: true, body: {} });
    auth.clear();
    auth.accessToken = "new-token";
    resolveFirst(jsonResponse(401, { code: "token_expired", error: "expired" }));

    await expect(pending).rejects.toMatchObject({ status: 401 });
    expect(fetchMock).toHaveBeenCalledOnce();
    expect(refresh).not.toHaveBeenCalled();
  });

  it("shares one refresh across concurrent 401 responses", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { code: "token_expired", error: "expired" }))
      .mockResolvedValueOnce(jsonResponse(401, { code: "token_expired", error: "expired" }))
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

  it("does not share a refresh across auth generations", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { code: "token_expired", error: "expired" }))
      .mockResolvedValueOnce(jsonResponse(401, { code: "token_expired", error: "expired" }));
    vi.stubGlobal("fetch", fetchMock);
    auth.accessToken = "old-token";

    const resolveRefreshes: Array<(refreshed: boolean) => void> = [];
    const refresh = vi.spyOn(auth, "tryRefresh").mockImplementation(
      () =>
        new Promise<boolean>((resolve) => {
          resolveRefreshes.push(resolve);
        }),
    );

    const oldRequest = request("/accounts/old", { authenticated: true });
    await vi.waitFor(() => expect(refresh).toHaveBeenCalledOnce());
    auth.clear();
    auth.accessToken = "new-token";
    const newRequest = request("/accounts/new", { authenticated: true });
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    await new Promise((resolve) => setTimeout(resolve, 0));

    resolveRefreshes.forEach((resolve) => resolve(false));
    await expect(oldRequest).rejects.toMatchObject({ status: 401 });
    await expect(newRequest).rejects.toMatchObject({ status: 401 });
    expect(refresh).toHaveBeenCalledTimes(2);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("does not retry when concurrent refresh fails", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { code: "token_expired", error: "expired" }))
      .mockResolvedValueOnce(jsonResponse(401, { code: "token_expired", error: "expired" }));
    vi.stubGlobal("fetch", fetchMock);
    const refresh = vi.spyOn(auth, "tryRefresh").mockResolvedValue(false);

    const results = await Promise.all([
      request<{ id: string }>("/accounts/a", { authenticated: true }).catch(
        (error: unknown) => error,
      ),
      request<{ id: string }>("/accounts/b", { authenticated: true }).catch(
        (error: unknown) => error,
      ),
    ]);

    expect(results).toEqual([expect.any(ApiError), expect.any(ApiError)]);
    expect(refresh).toHaveBeenCalledOnce();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("starts a new refresh for independent 401 cycles", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { code: "token_expired", error: "expired" }))
      .mockResolvedValueOnce(jsonResponse(200, { id: "a" }))
      .mockResolvedValueOnce(jsonResponse(401, { code: "token_expired", error: "expired" }))
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

describe("requestResponse", () => {
  beforeEach(() => {
    auth.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("returns an authenticated successful response with its body unread", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { id: "a1" }));
    vi.stubGlobal("fetch", fetchMock);
    auth.accessToken = "access-token";

    const response = await requestResponse("/accounts/a1", { authenticated: true });

    expect(response.bodyUsed).toBe(false);
    expect(await response.json()).toEqual({ id: "a1" });
    expect(fetchMock.mock.calls[0][1]).toMatchObject({
      headers: { Authorization: "Bearer access-token" },
    });
  });

  it("refreshes once after a 401 and returns the retry body unread", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { code: "token_expired", error: "expired" }))
      .mockResolvedValueOnce(jsonResponse(200, { ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    auth.accessToken = "expired-token";
    const refresh = vi.spyOn(auth, "tryRefresh").mockImplementation(() => {
      auth.accessToken = "refreshed-token";
      return Promise.resolve(true);
    });

    const response = await requestResponse("/accounts", { authenticated: true });

    expect(response.bodyUsed).toBe(false);
    expect(await response.json()).toEqual({ ok: true });
    expect(refresh).toHaveBeenCalledOnce();
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[1][1]).toMatchObject({
      headers: { Authorization: "Bearer refreshed-token" },
    });
  });

  it("throws ApiError after decoding a final non-2xx response", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          jsonResponse(422, { code: "insufficient_balance", error: "private detail" }),
        ),
    );

    await expect(requestResponse("/transfers", { method: "POST", body: {} })).rejects.toMatchObject(
      {
        kind: "api",
        status: 422,
        code: "insufficient_balance",
      } satisfies Partial<ApiError>,
    );
  });

  it("classifies a non-JSON error response without retaining its text", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("Bad Gateway", { status: 502 })));

    const error = await requestResponse("/accounts", { authenticated: true }).catch(
      (reason: unknown) => reason,
    );

    expect(error).toMatchObject({
      kind: "api",
      status: 502,
      code: null,
    } satisfies Partial<ApiError>);
    expect(toMessage(error)).toBe("SimpleBank is temporarily unavailable. Please try again.");
  });
});

describe("toMessage", () => {
  it.each([
    ["invalid_credentials", "The username or password is incorrect."],
    ["email_verification_required", "Verify your email address before signing in."],
    ["username_exists", "That username is already taken."],
    ["email_exists", "An account with that email address already exists."],
    ["insufficient_balance", "You don't have enough money in this account."],
    ["destination_balance_limit_exceeded", "The destination account cannot receive this amount."],
    ["currency_mismatch", "Transfers require accounts with the same currency."],
    ["daily_limit_exceeded", "This transfer would exceed your daily transfer limit."],
    ["transfer_limit_exceeded", "This amount exceeds the limit for a single transfer."],
    ["same_account_transfer", "Choose two different accounts for this transfer."],
    ["idempotency_conflict", "This transfer request conflicts with an earlier request."],
    ["invalid_verification_link", "This verification link is invalid or has expired."],
    ["not_found", "The requested resource was not found."],
    ["forbidden", "You don't have permission to do that."],
  ])("formats the %s code safely", (code, message) => {
    expect(toMessage(new ApiError("api", 422, code))).toBe(message);
  });

  it("formats status fallbacks without using unknown code text", () => {
    expect(toMessage(new ApiError("api", 503, "database_password_leaked"))).toBe(
      "SimpleBank is temporarily unavailable. Please try again.",
    );
  });

  it("does not expose arbitrary Error messages", () => {
    expect(toMessage(new Error("private path"))).toBe("Something went wrong. Please try again.");
  });

  it("falls back for unknown values", () => {
    expect(toMessage("boom")).toBe("Something went wrong. Please try again.");
  });
});

describe("isRetryable", () => {
  it.each([
    new ApiError("network"),
    new ApiError("invalid_response", 200),
    new ApiError("session_unavailable"),
    new ApiError("api", 408),
    new ApiError("api", 429),
    new ApiError("api", 500),
    new ApiError("api", 503),
  ])("returns true for retryable failures", (error) => {
    expect(isRetryable(error)).toBe(true);
  });

  it.each([
    new ApiError("aborted"),
    new ApiError("api", 400),
    new ApiError("api", 401),
    new ApiError("api", 422),
    new Error("network-like text"),
  ])("returns false for non-retryable failures", (error) => {
    expect(isRetryable(error)).toBe(false);
  });
});
