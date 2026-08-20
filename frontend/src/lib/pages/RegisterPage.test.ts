import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/svelte";
import RegisterPage from "./RegisterPage.svelte";

describe("RegisterPage", () => {
  it("constrains password length and explains policy", () => {
    render(RegisterPage);

    const field = screen.getByLabelText("Password");

    expect(field).toHaveAttribute("minlength", "15");
    expect(field).toHaveAttribute("maxlength", "72");
    expect(screen.getByText("At least 15 characters.")).toBeInTheDocument();
  });
});
