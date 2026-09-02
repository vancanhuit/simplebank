import { describe, it, expect } from "vitest";
import { formatMoney, formatSignedMoney, parseAmountToMinor, fractionDigits } from "./money";

describe("formatMoney", () => {
  it("renders USD minor units with two fraction digits", () => {
    expect(formatMoney(48235, "USD")).toContain("482.35");
  });

  it("renders VND without fraction digits", () => {
    const result = formatMoney(34500000, "VND");
    expect(result).not.toContain(".");
  });
});

describe("formatSignedMoney", () => {
  it("prefixes credits with a plus", () => {
    expect(formatSignedMoney(1000, "USD").startsWith("+")).toBe(true);
  });

  it("prefixes debits with a minus sign", () => {
    expect(formatSignedMoney(-1000, "USD").startsWith("−")).toBe(true);
  });

  it("has no sign for zero", () => {
    const result = formatSignedMoney(0, "USD");
    expect(result.startsWith("+")).toBe(false);
    expect(result.startsWith("−")).toBe(false);
  });
});

describe("parseAmountToMinor", () => {
  it("converts major units to minor units for two-decimal currencies", () => {
    expect(parseAmountToMinor("12.50", "USD")).toBe(1250);
    expect(parseAmountToMinor("0.01", "EUR")).toBe(1);
  });

  it("treats VND as having no minor units", () => {
    expect(parseAmountToMinor("1500", "VND")).toBe(1500);
  });

  it("scales without binary-float error", () => {
    // 1.005 * 100 is 100.4999… in IEEE-754; integer scaling must stay exact.
    expect(parseAmountToMinor("1.00", "USD")).toBe(100);
    expect(parseAmountToMinor("0.10", "USD")).toBe(10);
  });

  it("rejects more precision than the currency supports", () => {
    expect(parseAmountToMinor("1.239", "USD")).toBeNull();
    expect(parseAmountToMinor("12.999", "USD")).toBeNull();
    expect(parseAmountToMinor("1.5", "VND")).toBeNull();
  });

  it("rejects empty, non-numeric, and non-positive input", () => {
    expect(parseAmountToMinor("", "USD")).toBeNull();
    expect(parseAmountToMinor("abc", "USD")).toBeNull();
    expect(parseAmountToMinor("0", "USD")).toBeNull();
    expect(parseAmountToMinor("0.00", "USD")).toBeNull();
    expect(parseAmountToMinor("-5", "USD")).toBeNull();
  });

  it("rejects non-decimal notations and separators", () => {
    expect(parseAmountToMinor("1e3", "USD")).toBeNull();
    expect(parseAmountToMinor("0x10", "USD")).toBeNull();
    expect(parseAmountToMinor("Infinity", "USD")).toBeNull();
    expect(parseAmountToMinor("1,000", "USD")).toBeNull();
  });

  it("rejects amounts beyond the exact-integer range", () => {
    expect(parseAmountToMinor("999999999999999999", "USD")).toBeNull();
  });

  it("accepts numeric input (number-type inputs bind as numbers)", () => {
    expect(parseAmountToMinor(12.5, "USD")).toBe(1250);
    expect(parseAmountToMinor(1500, "VND")).toBe(1500);
  });
});

describe("currency helpers", () => {
  it("reports fraction digits per currency", () => {
    expect(fractionDigits("USD")).toBe(2);
    expect(fractionDigits("VND")).toBe(0);
  });
});
