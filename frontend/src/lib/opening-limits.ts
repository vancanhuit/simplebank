import type { AccountOpeningLimits } from "./api/types";
import { formatMoney, fractionDigits, type Currency } from "./money";

export type { AccountOpeningLimits };

export function openingLimitFor(limits: AccountOpeningLimits, currency: Currency): number {
  return limits[currency] ?? 0;
}

export function openingLimitInputMax(limit: number, currency: Currency): string {
  const digits = fractionDigits(currency);
  if (digits === 0) {
    return String(limit);
  }

  const raw = String(limit).padStart(digits + 1, "0");
  return `${raw.slice(0, -digits)}.${raw.slice(-digits)}`;
}

export function validateOpeningBalance(
  balance: number,
  currency: Currency,
  limits: AccountOpeningLimits,
): string | null {
  const limit = openingLimitFor(limits, currency);
  return balance <= limit
    ? null
    : `Opening deposit exceeds the ${formatMoney(limit, currency)} maximum.`;
}
