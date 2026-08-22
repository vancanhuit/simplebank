import { request } from "../api/client";
import type {
  LoginResponse,
  RegisterInput,
  AcceptedResponse,
  RenewResponse,
  User,
} from "../api/types";
import { replaceNavigation } from "../router.svelte";

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

  /** Exchange the httpOnly refresh cookie for a fresh access token. Returns success. */
  async tryRefresh(): Promise<boolean> {
    const gen = this.#generation;
    try {
      const res = await request<RenewResponse | undefined>("/tokens/renew", {
        method: "POST",
      });
      if (!res) {
        if (this.#generation === gen) {
          this.clear();
        }
        return false;
      }
      // Only apply response if no logout or newer login occurred during request.
      if (this.#generation === gen) {
        this.accessToken = res.access_token;
        this.user = res.user;
        return true;
      }
      return false;
    } catch {
      // Only clear if generation unchanged (logout may have already cleared).
      if (this.#generation === gen) {
        this.clear();
      }
      return false;
    }
  }

  async logout(): Promise<void> {
    this.loggingOut = true;
    ++this.#generation;
    this.#clearState();
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

  clear(): void {
    this.#generation += 1;
    this.#clearState();
  }
}

export { AuthStore };
export const auth = new AuthStore();
