import { request, toMessage } from "../api/client";
import type { Account } from "../api/types";
import type { Currency } from "../money";

/**
 * Client-side cache of the signed-in user's accounts. It owns loading and error
 * state so pages can render skeletons and error states without duplicating the
 * fetch logic.
 */
class AccountsStore {
  items = $state<Account[]>([]);
  loading = $state(false);
  error = $state<string | null>(null);
  loaded = $state(false);

  /** Account id to preselect on the transfer form (set from an account card). */
  transferFromId = $state<string | null>(null);

  async load(): Promise<void> {
    this.loading = true;
    this.error = null;
    try {
      // size=100 is the API's per-page maximum; it comfortably covers a
      // personal set of accounts without pagination UI.
      this.items = await request<Account[]>("/accounts?page=1&size=100", {
        authenticated: true,
      });
      this.loaded = true;
    } catch (err) {
      this.error = toMessage(err);
    } finally {
      this.loading = false;
    }
  }

  async create(currency: Currency): Promise<Account> {
    const account = await request<Account>("/accounts", {
      method: "POST",
      authenticated: true,
      body: { currency },
    });
    this.items = [...this.items, account];
    return account;
  }

  get(id: string): Account | undefined {
    return this.items.find((account) => account.id === id);
  }

  reset(): void {
    this.items = [];
    this.loaded = false;
    this.error = null;
    this.transferFromId = null;
  }
}

export const accounts = new AccountsStore();
