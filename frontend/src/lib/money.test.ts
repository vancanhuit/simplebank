import { describe, it, expect } from "vitest";
import {
  formatMoney,
  formatSignedMoney,
  parseAmountToMinor,
  isCurrency,
  fractionDigits,
} from "./money";

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

  it("rounds to the nearest minor unit", () => {
    expect(parseAmountToMinor("1.239", "USD")).toBe(124);
  });

  it("rejects empty, non-numeric, and non-positive input", () => {
    expect(parseAmountToMinor("", "USD")).toBeNull();
    expect(parseAmountToMinor("abc", "USD")).toBeNull();
    expect(parseAmountToMinor("0", "USD")).toBeNull();
    expect(parseAmountToMinor("-5", "USD")).toBeNull();
  });
});

describe("currency helpers", () => {
  it("recognizes supported currencies", () => {
    expect(isCurrency("USD")).toBe(true);
    expect(isCurrency("GBP")).toBe(false);
  });

  it("reports fraction digits per currency", () => {
    expect(fractionDigits("USD")).toBe(2);
    expect(fractionDigits("VND")).toBe(0);
  });
});
