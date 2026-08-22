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
  #generation = 0;
  #loadSequence = 0;

  /** Account id to preselect on the transfer form (set from an account card). */
  transferFromId = $state<string | null>(null);

  async load(): Promise<void> {
    const generation = this.#generation;
    const sequence = ++this.#loadSequence;
    this.loading = true;
    this.error = null;
    try {
      // size=100 is the API's per-page maximum; it comfortably covers a
      // personal set of accounts without pagination UI.
      const items = await request<Account[]>("/accounts?page=1&size=100", {
        authenticated: true,
      });
      if (this.#generation !== generation || this.#loadSequence !== sequence) {
        return;
      }
      this.items = items;
      this.loaded = true;
    } catch (err) {
      if (this.#generation === generation && this.#loadSequence === sequence) {
        this.error = toMessage(err);
      }
    } finally {
      if (this.#generation === generation && this.#loadSequence === sequence) {
        this.loading = false;
      }
    }
  }

  async create(currency: Currency, balance = 0): Promise<Account> {
    const generation = this.#generation;
    const account = await request<Account>("/accounts", {
      method: "POST",
      authenticated: true,
      body: { currency, balance },
    });
    if (this.#generation === generation) {
      this.items = [...this.items, account];
    }
    return account;
  }

  get(id: string): Account | undefined {
    return this.items.find((account) => account.id === id);
  }

  reset(): void {
    this.#generation += 1;
    this.#loadSequence += 1;
    this.items = [];
    this.loading = false;
    this.loaded = false;
    this.error = null;
    this.transferFromId = null;
  }
}

export const accounts = new AccountsStore();
