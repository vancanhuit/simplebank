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
    const res = await request<LoginResponse>("/users/login", {
      method: "POST",
      body: { username, password },
    });
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
  async tryRefresh(): Promise<RefreshOutcome> {
    const gen = this.#generation;
    try {
      const res = await request<RenewResponse | undefined>("/tokens/renew", {
        method: "POST",
      });
      if (this.#generation !== gen) {
        return "stale";
      }
      if (!res) {
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
