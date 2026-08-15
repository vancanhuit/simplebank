import { describe, expect, it } from "vitest";
import { createRawSnippet } from "svelte";
import { render, screen } from "@testing-library/svelte";
import Button from "./Button.svelte";

describe("Button", () => {
  it("disables interaction and signals busy state while loading", () => {
    const children = createRawSnippet(() => ({ render: () => "Save" }));
    render(Button, { loading: true, children });

    const button = screen.getByRole("button", { name: "Save" });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("aria-busy", "true");
  });
});
