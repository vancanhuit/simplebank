import { ApiError, request } from "../api/client";
import type {
  LoginResponse,
  RegisterInput,
  AcceptedResponse,
  RenewResponse,
  User,
} from "../api/types";
import { replaceNavigation } from "../router.svelte";

export type RefreshOutcome = "refreshed" | "no_session" | "expired" | "unavailable" | "stale";

interface RefreshAttempt {
  generation: number;
  promise: Promise<RefreshOutcome>;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isUser(value: unknown): value is User {
  return (
    isRecord(value) &&
    typeof value.username === "string" &&
    typeof value.full_name === "string" &&
    typeof value.email === "string" &&
    typeof value.is_email_verified === "boolean" &&
    typeof value.created_at === "string"
  );
}

function isRenewResponse(value: unknown): value is RenewResponse {
  return (
    isRecord(value) &&
    typeof value.access_token === "string" &&
    typeof value.access_token_expires_at === "string" &&
    isUser(value.user)
  );
}

function isLoginResponse(value: unknown): value is LoginResponse {
  return (
    isRecord(value) &&
    isRenewResponse(value) &&
    value.user.is_email_verified &&
    typeof value.session_id === "string"
  );
}

/**
 * Authentication state backed by httpOnly cookie sessions. The access token
 * stays in memory only; the refresh token never touches the browser.
 */
class AuthStore {
  user = $state<User | null>(null);
  accessToken = $state<string | null>(null);
  /** True until the initial session-restore attempt has completed. */
  initializing = $state(true);
  /** True from local logout invalidation through server response handling and navigation. */
  loggingOut = $state(false);
  renewalUnavailable = $state(false);
  sessionExpired = $state(false);
  /** Monotonic generation to prevent race conditions. */
  #generation = 0;
  #refreshAttempt: RefreshAttempt | null = null;

  get generation(): number {
    return this.#generation;
  }

  get isAuthenticated(): boolean {
    return this.user !== null && this.accessToken !== null;
  }

  /** Restore a session from the httpOnly cookie on app start. */
  async init(): Promise<void> {
    // Migration: purge legacy browser-readable refresh token.
    try {
      localStorage.removeItem("simplebank.session");
    } catch {
      // Storage unavailable; no action needed.
    }

    try {
      await this.tryRefresh();
    } finally {
      this.initializing = false;
    }
  }

  async login(username: string, password: string): Promise<void> {
    if (this.loggingOut) {
      throw new Error("Wait for sign-out to finish before signing in.");
    }
    this.#resetRefreshState();
    const gen = ++this.#generation;
    const res: unknown = await request("/users/login", {
      method: "POST",
      body: { username, password },
    });
    if (!isLoginResponse(res)) {
      throw new ApiError("invalid_response", 200);
    }
    // Only apply response if no logout or newer login occurred during request.
    if (this.#generation === gen) {
      this.user = res.user;
      this.accessToken = res.access_token;
    }
  }

  async register(input: RegisterInput): Promise<AcceptedResponse> {
    return request<AcceptedResponse>("/users", { method: "POST", body: input });
  }

  /** Exchange the httpOnly refresh cookie for a fresh access token. */
  tryRefresh(): Promise<RefreshOutcome> {
    const gen = this.#generation;
    if (this.#refreshAttempt?.generation === gen) {
      return this.#refreshAttempt.promise;
    }

    const attempt: RefreshAttempt = {
      generation: gen,
      promise: Promise.resolve("stale"),
    };
    attempt.promise = this.#performRefresh(gen).finally(() => {
      if (this.#refreshAttempt === attempt) {
        this.#refreshAttempt = null;
      }
    });
    this.#refreshAttempt = attempt;
    return attempt.promise;
  }

  async #performRefresh(gen: number): Promise<RefreshOutcome> {
    try {
      const res: unknown = await request("/tokens/renew", {
        method: "POST",
      });
      if (this.#generation !== gen) {
        return "stale";
      }
      if (res === undefined) {
        this.#invalidateSession();
        return "no_session";
      }
      if (!isRenewResponse(res)) {
        throw new ApiError("invalid_response", 200);
      }
      if (!res.user.is_email_verified) {
        this.#invalidateSession();
        return "no_session";
      }

      this.accessToken = res.access_token;
      this.user = res.user;
      this.renewalUnavailable = false;
      return "refreshed";
    } catch (error) {
      if (this.#generation !== gen) {
        return "stale";
      }
      if (error instanceof ApiError && error.code === "email_verification_required") {
        this.#invalidateSession();
        return "no_session";
      }
      if (error instanceof ApiError && error.status === 401) {
        this.#invalidateSession();
        this.sessionExpired = true;
        return "expired";
      }

      this.renewalUnavailable = true;
      return "unavailable";
    }
  }

  retryRefresh(): Promise<RefreshOutcome> {
    return this.tryRefresh();
  }

  consumeSessionExpired(): boolean {
    const expired = this.sessionExpired;
    this.sessionExpired = false;
    return expired;
  }

  async logout(): Promise<void> {
    this.loggingOut = true;
    ++this.#generation;
    this.#clearState();
    this.#resetRefreshState();
    let logoutFailed = false;
    try {
      await request<void>("/users/logout", { method: "POST" });
    } catch {
      logoutFailed = true;
    } finally {
      replaceNavigation("/login", logoutFailed ? { logoutFailed: true } : {});
      this.loggingOut = false;
    }
  }

  #clearState(): void {
    this.user = null;
    this.accessToken = null;
  }

  #resetRefreshState(): void {
    this.renewalUnavailable = false;
    this.sessionExpired = false;
  }

  #invalidateSession(): void {
    this.#generation += 1;
    this.#clearState();
    this.#resetRefreshState();
  }

  clear(): void {
    this.#invalidateSession();
  }
}

export { AuthStore };
export const auth = new AuthStore();
