import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/svelte";
import TextField from "./TextField.svelte";

describe("TextField", () => {
  it("announces a validation error and associates it with the field", () => {
    render(TextField, {
      props: {
        id: "recipient",
        label: "Recipient account id",
        value: "",
        error: "Enter the recipient account id.",
      },
    });

    const field = screen.getByRole("textbox", { name: "Recipient account id" });
    const error = screen.getByRole("alert");

    expect(field).toHaveAttribute("aria-invalid", "true");
    expect(field).toHaveAttribute("aria-describedby", "recipient-error");
    expect(error).toHaveAttribute("id", "recipient-error");
    expect(error).toHaveTextContent("Enter the recipient account id.");
  });

  it("retains aria-invalid and error-first aria-describedby in error state", () => {
    render(TextField, {
      props: {
        id: "amount",
        label: "Amount",
        value: "",
        hint: "Enter an amount greater than zero.",
        error: "Amount is required.",
      },
    });

    const field = screen.getByRole("textbox", { name: "Amount" });

    expect(field).toHaveAttribute("aria-invalid", "true");
    expect(field).toHaveAttribute("aria-describedby", "amount-error amount-hint");
  });

  it("preserves disabled state semantics", () => {
    render(TextField, {
      props: {
        id: "deposit",
        label: "Deposit",
        value: "",
        disabled: true,
      },
    });

    const field = screen.getByRole("textbox", { name: "Deposit" });

    expect(field).toBeDisabled();
  });

  it("applies forced-colors invalid mapping with aria-invalid", () => {
    render(TextField, {
      props: {
        id: "recipient",
        label: "Recipient",
        value: "",
        error: "Invalid recipient.",
      },
    });

    const field = screen.getByRole("textbox", { name: "Recipient" });

    expect(field).toHaveAttribute("aria-invalid", "true");
    expect(field.className).toContain("forced-colors:aria-invalid:border-[Mark]");
  });
});
