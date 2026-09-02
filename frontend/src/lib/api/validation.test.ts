import { describe, expect, it } from "vitest";
import { ApiError } from "./client";
import {
  account,
  accountOpeningLimits,
  notificationPage,
  notificationReadResponse,
  transferLimits,
  transferResult,
  verificationResponse,
} from "./validation";

const validAccount = {
  id: "account-1",
  owner: "alice",
  balance: 100,
  currency: "USD",
  created_at: "2026-01-01T00:00:00Z",
};

describe("API response validation", () => {
  it("accepts the core account, transfer, notification, limits, mutation, and verification contracts", () => {
    expect(account(validAccount)).toEqual(validAccount);
    expect(accountOpeningLimits({ USD: 100 })).toEqual({ USD: 100 });
    expect(transferLimits({ USD: { max_per_transfer: 100, daily: 500 } })).toEqual({
      USD: { max_per_transfer: 100, daily: 500 },
    });
    expect(
      transferResult({
        transfer: {
          id: "transfer-1",
          from_account_id: "account-1",
          to_account_id: "account-2",
          amount: 50,
          idempotency_key: "key-1",
          created_at: "2026-01-01T00:00:00Z",
        },
        from_account: validAccount,
      }),
    ).toHaveProperty("transfer.id", "transfer-1");
    expect(notificationPage({ notifications: [], unread_count: 0, next_cursor: null })).toEqual({
      notifications: [],
      unread_count: 0,
      next_cursor: null,
    });
    expect(notificationReadResponse({ unread_count: 1 })).toEqual({ unread_count: 1 });
    expect(verificationResponse({ is_verified: true })).toEqual({ is_verified: true });
  });

  it.each([
    () => account({ ...validAccount, balance: "100" }),
    () => account({ ...validAccount, balance: -1 }),
    () => account({ ...validAccount, id: "" }),
    () => account({ ...validAccount, created_at: "not-a-date" }),
    () => accountOpeningLimits({ GBP: 100 }),
    () => transferLimits({ USD: { max_per_transfer: -1, daily: 500 } }),
    () => transferLimits({ GBP: { max_per_transfer: 100, daily: 500 } }),
    () =>
      transferResult({
        transfer: {
          id: "transfer-1",
          from_account_id: "account-1",
          to_account_id: "account-2",
          amount: 0,
          idempotency_key: "key-1",
          created_at: "2026-01-01T00:00:00Z",
        },
        from_account: validAccount,
      }),
    () => notificationPage({ notifications: [], unread_count: "0", next_cursor: null }),
    () => notificationPage({ notifications: [], unread_count: 0, next_cursor: "" }),
    () => notificationReadResponse({ unread_count: -1 }),
    () => verificationResponse({ is_verified: "yes" }),
  ])("rejects malformed successful payloads", (validate) => {
    expect(validate).toThrowError(ApiError);
    try {
      validate();
    } catch (error) {
      expect(error).toMatchObject({ kind: "invalid_response", status: 200 });
    }
  });
});
