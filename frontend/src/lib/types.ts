import type { Currency } from "./money";

/** A bank account, mirroring the backend `Account` model. */
export interface Account {
  id: string;
  owner: string;
  balance: number; // minor units
  currency: Currency;
}

/** A ledger movement shown in the activity feed, resolved for display. */
export interface ActivityItem {
  id: string;
  /** Positive when money arrives, negative when it leaves. */
  amount: number; // minor units, signed
  currency: Currency;
  counterparty: string;
  /** ISO 8601 timestamp. */
  occurredAt: string;
}
