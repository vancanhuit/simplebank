import { request } from "../api/client";
import type { LoginResponse, RegisterInput, RenewResponse, User } from "../api/types";
import { navigate } from "../router.svelte";

const STORAGE_KEY = "simplebank.session";

interface PersistedSession {
  refreshToken: string;
  user: User;
}

/**
 * Persist only the refresh token and user profile. The short-lived access token
 * stays in memory. Storing the refresh token in localStorage is the pragmatic
 * choice for this SPA because the API returns tokens in JSON rather than
 * httpOnly cookies; the backend stores only a hash of the token at rest.
 */
function loadSession(): PersistedSession | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return null;
    }
    const parsed = JSON.parse(raw) as Partial<PersistedSession>;
    if (typeof parsed.refreshToken === "string" && parsed.user) {
      return { refreshToken: parsed.refreshToken, user: parsed.user };
    }
  } catch {
    // Corrupt or unavailable storage: treat as no session.
  }
  return null;
}

function saveSession(session: PersistedSession | null): void {
  try {
    if (session) {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(session));
    } else {
      localStorage.removeItem(STORAGE_KEY);
    }
  } catch {
    // Storage may be disabled (private mode); the in-memory session still works.
  }
}

class AuthStore {
  user = $state<User | null>(null);
  accessToken = $state<string | null>(null);
  /** True until the initial session-restore attempt has completed. */
  initializing = $state(true);

  #refreshToken: string | null = null;

  get isAuthenticated(): boolean {
    return this.user !== null && this.accessToken !== null;
  }

  get canRefresh(): boolean {
    return this.#refreshToken !== null;
  }

  /** Restore a session from storage on app start, refreshing the access token. */
  async init(): Promise<void> {
    const session = loadSession();
    if (session) {
      this.#refreshToken = session.refreshToken;
      this.user = session.user;
      const ok = await this.tryRefresh();
      if (!ok) {
        this.clear();
      }
    }
    this.initializing = false;
  }

  async login(username: string, password: string): Promise<void> {
    const res = await request<LoginResponse>("/users/login", {
      method: "POST",
      body: { username, password },
    });
    this.user = res.user;
    this.accessToken = res.access_token;
    this.#refreshToken = res.refresh_token;
    saveSession({ refreshToken: res.refresh_token, user: res.user });
  }

  async register(input: RegisterInput): Promise<User> {
    return request<User>("/users", { method: "POST", body: input });
  }

  /** Exchange the refresh token for a fresh access token. Returns success. */
  async tryRefresh(): Promise<boolean> {
    if (!this.#refreshToken) {
      return false;
    }
    try {
      const res = await request<RenewResponse>("/tokens/renew", {
        method: "POST",
        body: { refresh_token: this.#refreshToken },
      });
      this.accessToken = res.access_token;
      return true;
    } catch {
      return false;
    }
  }

  logout(): void {
    this.clear();
    navigate("/login");
  }

  clear(): void {
    this.user = null;
    this.accessToken = null;
    this.#refreshToken = null;
    saveSession(null);
  }
}

export const auth = new AuthStore();
