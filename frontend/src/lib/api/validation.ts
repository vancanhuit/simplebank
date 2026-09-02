import { ApiError } from "./client";
import type {
  AcceptedResponse,
  Account,
  AccountOpeningLimits,
  Notification,
  NotificationPage,
  NotificationReadResponse,
  Transfer,
  TransferLimits,
  TransferResult,
} from "./types";
import { CURRENCIES, type Currency } from "../money";

function record(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function string(value: unknown): value is string {
  return typeof value === "string";
}

function nonempty(value: unknown): value is string {
  return string(value) && value.length > 0;
}

function timestamp(value: unknown): value is string {
  return nonempty(value) && !Number.isNaN(Date.parse(value));
}

function integer(value: unknown): value is number {
  return Number.isSafeInteger(value);
}

function currency(value: unknown): value is Currency {
  return string(value) && CURRENCIES.some((code) => code === value);
}

function invalid(): never {
  throw new ApiError("invalid_response", 200);
}

export function account(value: unknown): Account {
  const data = record(value);
  if (
    !data ||
    !nonempty(data.id) ||
    !nonempty(data.owner) ||
    !integer(data.balance) ||
    data.balance < 0 ||
    !currency(data.currency) ||
    !timestamp(data.created_at)
  ) {
    return invalid();
  }
  return data as unknown as Account;
}

export function accounts(value: unknown): Account[] {
  if (!Array.isArray(value)) return invalid();
  return value.map(account);
}

export function transfer(value: unknown): Transfer {
  const data = record(value);
  if (
    !data ||
    !nonempty(data.id) ||
    !nonempty(data.from_account_id) ||
    !nonempty(data.to_account_id) ||
    !integer(data.amount) ||
    data.amount <= 0 ||
    !nonempty(data.idempotency_key) ||
    !timestamp(data.created_at)
  ) {
    return invalid();
  }
  return data as unknown as Transfer;
}

export function transfers(value: unknown): Transfer[] {
  if (!Array.isArray(value)) return invalid();
  return value.map(transfer);
}

export function transferResult(value: unknown): TransferResult {
  const data = record(value);
  if (!data) return invalid();
  return { transfer: transfer(data.transfer), from_account: account(data.from_account) };
}

export function accountOpeningLimits(value: unknown): AccountOpeningLimits {
  const data = record(value);
  if (!data) return invalid();
  for (const [code, limit] of Object.entries(data)) {
    if (!currency(code) || !integer(limit) || limit < 0) return invalid();
  }
  return data;
}

export function transferLimits(value: unknown): TransferLimits {
  const data = record(value);
  if (!data) return invalid();
  for (const [code, limit] of Object.entries(data)) {
    const fields = record(limit);
    if (
      !currency(code) ||
      !fields ||
      !integer(fields.max_per_transfer) ||
      fields.max_per_transfer < 0 ||
      !integer(fields.daily) ||
      fields.daily < 0
    ) {
      return invalid();
    }
  }
  return data as TransferLimits;
}

export function notification(value: unknown): Notification {
  const data = record(value);
  if (
    !data ||
    !nonempty(data.id) ||
    !nonempty(data.account_id) ||
    !nonempty(data.transfer_id) ||
    (data.direction !== "sent" && data.direction !== "received") ||
    !integer(data.amount) ||
    data.amount <= 0 ||
    !currency(data.currency) ||
    !integer(data.balance) ||
    data.balance < 0 ||
    (data.read_at !== null && !timestamp(data.read_at)) ||
    !timestamp(data.created_at)
  ) {
    return invalid();
  }
  return data as unknown as Notification;
}

export function notificationPage(value: unknown): NotificationPage {
  const data = record(value);
  if (
    !data ||
    !Array.isArray(data.notifications) ||
    !integer(data.unread_count) ||
    data.unread_count < 0 ||
    (data.next_cursor !== null && !nonempty(data.next_cursor))
  ) {
    return invalid();
  }
  return {
    notifications: data.notifications.map(notification),
    unread_count: data.unread_count,
    next_cursor: data.next_cursor,
  };
}

export function notificationReadResponse(value: unknown): NotificationReadResponse {
  const data = record(value);
  if (!data || !integer(data.unread_count) || data.unread_count < 0) return invalid();
  return data as unknown as NotificationReadResponse;
}

export function acceptedResponse(value: unknown): AcceptedResponse {
  const data = record(value);
  if (!data || !string(data.message)) return invalid();
  return data as unknown as AcceptedResponse;
}

export function verificationResponse(value: unknown): { is_verified: boolean } {
  const data = record(value);
  if (!data || typeof data.is_verified !== "boolean") return invalid();
  return { is_verified: data.is_verified };
}
