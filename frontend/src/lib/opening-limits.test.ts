import { describe, expect, it } from "vitest";
import { openingLimitFor, openingLimitInputMax, validateOpeningBalance } from "./opening-limits";

describe("opening limits", () => {
  const limits = { USD: 100000, EUR: 100000, VND: 25000000 };

  it("treats a missing currency as a zero cap", () => {
    expect(openingLimitFor({}, "USD")).toBe(0);
  });

  it("accepts the boundary and rejects one minor unit above it", () => {
    expect(validateOpeningBalance(100000, "USD", limits)).toBeNull();
    expect(validateOpeningBalance(100001, "USD", limits)).toContain("maximum");
  });

  it("formats input max without floating-point rounding", () => {
    expect(openingLimitInputMax(100000, "USD")).toBe("1000.00");
    expect(openingLimitInputMax(25000000, "VND")).toBe("25000000");
  });
});
