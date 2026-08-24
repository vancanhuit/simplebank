import { auth, type RefreshOutcome } from "../stores/auth.svelte";

/** All API routes live under this same-origin prefix. In development the Vite
 *  dev server proxies it to the Go backend (see vite.config.ts). */
const BASE_URL = "/api/v1";

interface RefreshAttempt {
  generation: number;
  promise: Promise<RefreshOutcome>;
}

let refreshAttempt: RefreshAttempt | null = null;

function refreshAccessToken(generation: number): Promise<RefreshOutcome> {
  if (refreshAttempt?.generation === generation) {
    return refreshAttempt.promise;
  }

  const attempt: RefreshAttempt = {
    generation,
    promise: Promise.resolve("stale"),
  };
  attempt.promise = auth.tryRefresh().finally(() => {
    if (refreshAttempt === attempt) {
      refreshAttempt = null;
    }
  });
  refreshAttempt = attempt;
  return attempt.promise;
}

export type ApiErrorKind =
  "api" | "network" | "invalid_response" | "aborted" | "session_unavailable";

/** Classified API client failure. Server and native error text is never retained. */
export class ApiError extends Error {
  constructor(
    readonly kind: ApiErrorKind,
    readonly status: number | null = null,
    readonly code: string | null = null,
    readonly retryAfterSeconds: number | null = null,
  ) {
    super(code ?? kind);
    this.name = "ApiError";
  }
}

export interface RequestOptions {
  method?: "GET" | "POST" | "PUT" | "DELETE";
  /** JSON-serializable request body. */
  body?: unknown;
  /** Attach the access token as a Bearer credential. */
  authenticated?: boolean;
  signal?: AbortSignal;
}

async function send(path: string, options: RequestOptions): Promise<Response> {
  const headers: Record<string, string> = {};
  if (options.body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  if (options.authenticated && auth.accessToken) {
    headers["Authorization"] = `Bearer ${auth.accessToken}`;
  }
  try {
    return await fetch(`${BASE_URL}${path}`, {
      method: options.method ?? "GET",
      headers,
      body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
      credentials: "same-origin",
      cache: "no-store",
      signal: options.signal,
    });
  } catch (error) {
    const kind =
      typeof error === "object" && error !== null && "name" in error && error.name === "AbortError"
        ? "aborted"
        : "network";
    throw new ApiError(kind);
  }
}

/** Perform an API request without consuming a successful response body. */
export async function requestResponse(
  path: string,
  options: RequestOptions = {},
): Promise<Response> {
  const generation = auth.generation;
  let response = await send(path, options);

  if (response.status === 401 && options.authenticated && auth.generation === generation) {
    const outcome = await refreshAccessToken(generation);
    if (outcome === "refreshed" && auth.generation === generation) {
      response = await send(path, options);
    } else if (outcome === "unavailable") {
      throw new ApiError("session_unavailable");
    }
  }

  if (!response.ok) {
    await decode(response);
  }
  return response;
}

/**
 * Perform an API request and decode the JSON response. On a 401 for an
 * authenticated request, it transparently refreshes the access token once and
 * retries, so an expired access token is invisible to callers.
 */
export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  return decode<T>(await requestResponse(path, options));
}

async function decode<T>(response: Response): Promise<T> {
  let text: string;
  try {
    text = await response.text();
  } catch {
    throw new ApiError(response.ok ? "invalid_response" : "api", response.status);
  }

  let data: unknown;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      if (response.ok) {
        throw new ApiError("invalid_response", response.status);
      }
    }
  }

  if (!response.ok) {
    const code = apiCode(data);
    const retryAfterSeconds =
      response.status === 429 ? parseRetryAfter(response.headers.get("Retry-After")) : null;
    throw new ApiError("api", response.status, code, retryAfterSeconds);
  }
  return data as T;
}

function apiCode(data: unknown): string | null {
  if (data && typeof data === "object" && "code" in data) {
    const code = data.code;
    if (typeof code === "string" && code.trim().length > 0) {
      return code;
    }
  }
  return null;
}

function parseRetryAfter(value: string | null): number | null {
  if (value === null) {
    return null;
  }
  const seconds = Number.parseInt(value, 10);
  return seconds > 0 ? seconds : null;
}

/** Extract a user-facing message from any thrown value. */
export function toMessage(error: unknown): string {
  if (!(error instanceof ApiError)) {
    return "Something went wrong. Please try again.";
  }

  switch (error.code) {
    case "invalid_credentials":
      return "The username or password is incorrect.";
    case "email_verification_required":
      return "Verify your email address before signing in.";
    case "username_exists":
      return "That username is already taken.";
    case "email_exists":
      return "An account with that email address already exists.";
    case "insufficient_balance":
      return "You don't have enough money in this account.";
    case "destination_balance_limit_exceeded":
      return "The destination account cannot receive this amount.";
    case "currency_mismatch":
      return "Transfers require accounts with the same currency.";
    case "daily_limit_exceeded":
      return "This transfer would exceed your daily transfer limit.";
    case "transfer_limit_exceeded":
      return "This amount exceeds the limit for a single transfer.";
    case "same_account_transfer":
      return "Choose two different accounts for this transfer.";
    case "idempotency_conflict":
      return "This transfer request conflicts with an earlier request.";
    case "invalid_verification_link":
      return "This verification link is invalid or has expired.";
    case "not_found":
      return "The requested resource was not found.";
    case "forbidden":
      return "You don't have permission to do that.";
  }

  switch (error.kind) {
    case "network":
      return "We couldn't reach SimpleBank. Check your connection and try again.";
    case "invalid_response":
      return "SimpleBank returned an unexpected response. Please try again.";
    case "aborted":
      return "";
    case "session_unavailable":
      return "Your session could not be restored. Please try again.";
    case "api":
      if (error.status === 429) {
        return error.retryAfterSeconds === null
          ? "Too many attempts. Please try again later."
          : `Too many attempts. Try again in ${error.retryAfterSeconds} ${error.retryAfterSeconds === 1 ? "second" : "seconds"}.`;
      }
      if (error.status !== null && error.status >= 500) {
        return "SimpleBank is temporarily unavailable. Please try again.";
      }
      if (error.status === 408) {
        return "The request timed out. Please try again.";
      }
      if (error.status === 401) {
        return "Your session has expired. Please sign in again.";
      }
      if (error.status === 403) {
        return "You don't have permission to do that.";
      }
      if (error.status === 404) {
        return "The requested resource was not found.";
      }
      return "We couldn't complete your request. Please check your details and try again.";
  }
}

/** Whether retrying the classified failure may succeed without changing the request. */
export function isRetryable(error: unknown): boolean {
  if (!(error instanceof ApiError)) {
    return false;
  }
  if (
    error.kind === "network" ||
    error.kind === "invalid_response" ||
    error.kind === "session_unavailable"
  ) {
    return true;
  }
  return (
    error.kind === "api" &&
    (error.status === 408 || error.status === 429 || (error.status !== null && error.status >= 500))
  );
}
