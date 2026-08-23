import { auth } from "../stores/auth.svelte";

/** All API routes live under this same-origin prefix. In development the Vite
 *  dev server proxies it to the Go backend (see vite.config.ts). */
const BASE_URL = "/api/v1";

interface RefreshAttempt {
  generation: number;
  promise: Promise<boolean>;
}

let refreshAttempt: RefreshAttempt | null = null;

function refreshAccessToken(generation: number): Promise<boolean> {
  if (refreshAttempt?.generation === generation) {
    return refreshAttempt.promise;
  }

  const attempt: RefreshAttempt = {
    generation,
    promise: Promise.resolve(false),
  };
  attempt.promise = auth.tryRefresh().finally(() => {
    if (refreshAttempt === attempt) {
      refreshAttempt = null;
    }
  });
  refreshAttempt = attempt;
  return attempt.promise;
}

/** Error thrown for any non-2xx API response. Carries the HTTP status and the
 *  server's client-safe `{"error": "..."}` message. */
export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
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
  return fetch(`${BASE_URL}${path}`, {
    method: options.method ?? "GET",
    headers,
    body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
    credentials: "same-origin",
    cache: "no-store",
    signal: options.signal,
  });
}

/** Perform an API request without consuming a successful response body. */
export async function requestResponse(
  path: string,
  options: RequestOptions = {},
): Promise<Response> {
  const generation = auth.generation;
  let response = await send(path, options);

  if (response.status === 401 && options.authenticated && auth.generation === generation) {
    const refreshed = await refreshAccessToken(generation);
    if (refreshed && auth.generation === generation) {
      response = await send(path, options);
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
  const text = await response.text();
  const data: unknown = text ? JSON.parse(text) : undefined;

  if (!response.ok) {
    if (response.status === 429) {
      const retryAfter = response.headers.get("Retry-After");
      const seconds = retryAfter === null ? 0 : Number.parseInt(retryAfter, 10);
      const wait =
        seconds > 0
          ? ` Try again in ${seconds} ${seconds === 1 ? "second" : "seconds"}.`
          : " Please try again later.";
      throw new ApiError(response.status, `Too many attempts.${wait}`);
    }
    throw new ApiError(response.status, errorMessage(data, response.status));
  }
  return data as T;
}

function errorMessage(data: unknown, status: number): string {
  if (data && typeof data === "object" && "error" in data) {
    const value = data.error;
    if (typeof value === "string" && value.length > 0) {
      return value;
    }
  }
  return `Request failed (${status})`;
}

/** Extract a user-facing message from any thrown value. */
export function toMessage(error: unknown): string {
  if (error instanceof ApiError) {
    return error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return "Something went wrong. Please try again.";
}
