import { describe, expect, it } from "vitest";
import { validateLogin, validateRegistration } from "./auth-validation";

describe("validateLogin", () => {
  it("rejects whitespace-only required fields", () => {
    expect(validateLogin({ username: "   ", password: "" })).toEqual({
      values: { username: "", password: "" },
      errors: {
        username: "Enter your username.",
        password: "Enter your password.",
      },
    });
  });

  it("rejects a username containing non-ASCII-alphanumeric characters", () => {
    expect(validateLogin({ username: "alice_01", password: "secret" }).errors).toEqual({
      username: "Use letters and numbers only.",
    });
  });

  it("normalizes the username without trimming the password", () => {
    expect(validateLogin({ username: " alice01 ", password: " secret " })).toEqual({
      values: { username: "alice01", password: " secret " },
      errors: {},
    });
  });
});

describe("validateRegistration", () => {
  it("rejects whitespace-only required fields", () => {
    expect(
      validateRegistration({ fullName: "  ", username: "  ", email: "  ", password: "" }),
    ).toEqual({
      values: { fullName: "", username: "", email: "", password: "" },
      errors: {
        fullName: "Enter your full name.",
        username: "Enter your username.",
        email: "Enter a valid email address.",
        password: "Enter your password.",
      },
    });
  });

  it("rejects a username containing non-ASCII-alphanumeric characters", () => {
    const result = validateRegistration({
      fullName: "Alice Smith",
      username: "álîce",
      email: "alice@example.com",
      password: "correct horse battery staple",
    });

    expect(result.errors).toEqual({ username: "Use letters and numbers only." });
  });

  it("rejects an invalid email shape", () => {
    const result = validateRegistration({
      fullName: "Alice Smith",
      username: "alice01",
      email: "alice@example",
      password: "correct horse battery staple",
    });

    expect(result.errors).toEqual({ email: "Enter a valid email address." });
  });

  it("rejects a 14-character password", () => {
    const result = validateRegistration({
      fullName: "Alice Smith",
      username: "alice01",
      email: "alice@example.com",
      password: "12345678901234",
    });

    expect(result.errors).toEqual({ password: "Use at least 15 characters." });
  });

  it("counts Unicode code points when enforcing the character minimum", () => {
    const result = validateRegistration({
      fullName: "Alice Smith",
      username: "alice01",
      email: "alice@example.com",
      password: "🙂".repeat(14),
    });

    expect(result.errors).toEqual({ password: "Use at least 15 characters." });
  });

  it("rejects a password over 72 UTF-8 bytes", () => {
    const result = validateRegistration({
      fullName: "Alice Smith",
      username: "alice01",
      email: "alice@example.com",
      password: "🙂".repeat(19),
    });

    expect(result.errors).toEqual({ password: "Use no more than 72 bytes." });
  });

  it("normalizes textual values without trimming the password", () => {
    expect(
      validateRegistration({
        fullName: " Alice Smith ",
        username: " alice01 ",
        email: " alice@example.com ",
        password: "correct horse battery staple",
      }),
    ).toEqual({
      values: {
        fullName: "Alice Smith",
        username: "alice01",
        email: "alice@example.com",
        password: "correct horse battery staple",
      },
      errors: {},
    });
  });
});
