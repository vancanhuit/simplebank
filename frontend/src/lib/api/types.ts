import type { Currency } from "../money";

/** A bank account, mirroring the backend `Account` model. */
export interface Account {
  id: string;
  owner: string;
  balance: number; // minor units (int64 on the server)
  currency: Currency;
  created_at: string; // ISO 8601
}

/** The authenticated user, mirroring the backend `userResponse`. */
export interface User {
  username: string;
  full_name: string;
  email: string;
  is_email_verified: boolean;
  created_at: string;
}

/** Response from `POST /users/login`. */
export interface LoginResponse {
  access_token: string;
  access_token_expires_at: string;
  session_id: string;
  user: User;
}

/** Response from `POST /tokens/renew`. */
export interface RenewResponse {
  access_token: string;
  access_token_expires_at: string;
  user: User;
}

/** Response from `GET /account-opening-limits`: currency code → maximum opening
 *  deposit in minor units. Missing currencies are treated as a zero cap. */
export type AccountOpeningLimits = Partial<Record<Currency, number>>;

/** Generic accepted response for async operations. */
export interface AcceptedResponse {
  message: string;
}

/** Payload for `POST /users`. */
export interface RegisterInput {
  username: string;
  password: string;
  full_name: string;
  email: string;
}

/** A ledger movement, mirroring the backend `Transfer` model. */
export interface Transfer {
  id: string;
  from_account_id: string;
  to_account_id: string;
  amount: number; // minor units
  idempotency_key: string;
  created_at: string;
}

/** Response from `POST /transfers`. */
export interface TransferResult {
  transfer: Transfer;
  from_account: Account;
}

/** Per-currency transfer ceilings, mirroring the backend `config.CurrencyLimit`.
 *  Amounts are in the currency's minor units; 0 means the limit is disabled. */
export interface CurrencyLimit {
  max_per_transfer: number;
  daily: number;
}

/** Response from `GET /transfer-limits`: currency code → ceilings. Currencies
 *  without limits are simply absent from the map. */
export type TransferLimits = Record<string, CurrencyLimit>;

export type NotificationDirection = "sent" | "received";

export interface Notification {
  id: string;
  account_id: string;
  transfer_id: string;
  direction: NotificationDirection;
  amount: number;
  currency: Currency;
  balance: number;
  read_at: string | null;
  created_at: string;
}

export interface NotificationPage {
  notifications: Notification[];
  unread_count: number;
  next_cursor: string | null;
}

export interface NotificationReadResponse {
  unread_count: number;
}
