/** Supported currencies mirror the backend `internal/currency` package. */
export type Currency = "USD" | "EUR" | "VND";

/** All currencies the UI can operate on, in display order. */
export const CURRENCIES: readonly Currency[] = ["USD", "EUR", "VND"];

/** Number of minor units per major unit (e.g. cents per dollar). */
const MINOR_UNITS: Record<Currency, number> = {
  USD: 100,
  EUR: 100,
  VND: 1,
};

/** Fraction digits used when rendering each currency. */
const FRACTION_DIGITS: Record<Currency, number> = {
  USD: 2,
  EUR: 2,
  VND: 0,
};

/** Whether a string is one of the supported currency codes. */
export function isCurrency(value: string): value is Currency {
  return (CURRENCIES as readonly string[]).includes(value);
}

/** Number of fraction digits rendered for a currency (0 for VND). */
export function fractionDigits(currency: Currency): number {
  return FRACTION_DIGITS[currency];
}

/**
 * Format a balance stored in minor units (the backend stores `balance` as an
 * int64 of the smallest currency unit) into a localized currency string.
 */
export function formatMoney(minorAmount: number, currency: Currency): string {
  const major = minorAmount / MINOR_UNITS[currency];
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency,
    minimumFractionDigits: FRACTION_DIGITS[currency],
    maximumFractionDigits: FRACTION_DIGITS[currency],
  }).format(major);
}

/** Format a signed transfer amount, always showing the leading + or −. */
export function formatSignedMoney(minorAmount: number, currency: Currency): string {
  const sign = minorAmount > 0 ? "+" : minorAmount < 0 ? "−" : "";
  return `${sign}${formatMoney(Math.abs(minorAmount), currency)}`;
}

/**
 * Convert a user-entered major-unit amount (e.g. "12.50") into integer minor
 * units for the API. Returns null when the input is empty, not a finite number,
 * or not strictly positive, so callers can surface a validation error.
 */
export function parseAmountToMinor(input: string, currency: Currency): number | null {
  const trimmed = input.trim();
  if (trimmed === "") {
    return null;
  }
  const value = Number(trimmed);
  if (!Number.isFinite(value) || value <= 0) {
    return null;
  }
  const minor = Math.round(value * MINOR_UNITS[currency]);
  return minor > 0 ? minor : null;
}
