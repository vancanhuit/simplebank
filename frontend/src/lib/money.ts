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
 * units for the API. Returns null when the input is empty, malformed, carries
 * more fraction digits than the currency supports, is not strictly positive, or
 * exceeds the range JavaScript can represent exactly, so callers can surface a
 * validation error.
 *
 * Scaling is done with integer string math rather than `value * MINOR_UNITS`
 * to avoid binary-float error (e.g. `1.005 * 100` is `100.4999…`, which would
 * round *down* to 100). Excess precision is rejected instead of silently
 * rounded away (`"12.999"` USD would otherwise become $13.00).
 *
 * Accepts a number as well as a string because a `<input type="number">` bound
 * with `bind:value` yields a number at runtime.
 */
export function parseAmountToMinor(input: string | number, currency: Currency): number | null {
  const trimmed = String(input).trim();
  // Strict base-10 decimal: no sign, exponent, or separators. Rejects "",
  // "abc", "1e3", "0x10", "Infinity", "-5".
  const match = /^(\d+)(?:\.(\d+))?$/.exec(trimmed);
  if (!match) {
    return null;
  }
  const [, intPart, fracPart = ""] = match;
  const digits = FRACTION_DIGITS[currency];
  if (fracPart.length > digits) {
    return null;
  }
  // Left-justify the fraction to the currency's precision, then concatenate to
  // form the minor-unit integer as a string (avoids any float multiplication).
  const combined = intPart + fracPart.padEnd(digits, "0");
  const minor = Number(combined);
  if (!Number.isSafeInteger(minor) || minor <= 0) {
    return null;
  }
  return minor;
}
