import { auth } from "../stores/auth.svelte";

/** All API routes live under this same-origin prefix. In development the Vite
 *  dev server proxies it to the Go backend (see vite.config.ts). */
const BASE_URL = "/api/v1";

let refreshPromise: Promise<boolean> | null = null;

function refreshAccessToken(): Promise<boolean> {
  refreshPromise ??= auth.tryRefresh().finally(() => {
    refreshPromise = null;
  });
  return refreshPromise;
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

/**
 * Perform an API request and decode the JSON response. On a 401 for an
 * authenticated request, it transparently refreshes the access token once and
 * retries, so an expired access token is invisible to callers.
 */
export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  let response = await send(path, options);

  if (response.status === 401 && options.authenticated) {
    const refreshed = await refreshAccessToken();
    if (refreshed) {
      response = await send(path, options);
    }
  }

  return decode<T>(response);
}

async function decode<T>(response: Response): Promise<T> {
  const text = await response.text();
  const data: unknown = text ? JSON.parse(text) : undefined;

  if (!response.ok) {
    throw new ApiError(response.status, errorMessage(data, response.status));
  }
  return data as T;
}

function errorMessage(data: unknown, status: number): string {
  if (data && typeof data === "object" && "error" in data) {
    const value = (data as { error: unknown }).error;
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
