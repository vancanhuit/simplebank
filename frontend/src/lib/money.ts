/** Supported currencies mirror the backend `internal/currency` package. */
export type Currency = "USD" | "EUR" | "VND";

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
