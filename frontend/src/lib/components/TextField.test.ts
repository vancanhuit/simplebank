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
});
