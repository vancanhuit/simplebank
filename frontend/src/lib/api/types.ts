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
  refresh_token: string;
  refresh_token_expires_at: string;
  session_id: string;
  user: User;
}

/** Response from `POST /tokens/renew`. */
export interface RenewResponse {
  access_token: string;
  access_token_expires_at: string;
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
  created_at: string;
}

/** Response from `POST /transfers`, mirroring `TransferTxResult`. */
export interface TransferResult {
  transfer: Transfer;
  from_account: Account;
  to_account: Account;
}
