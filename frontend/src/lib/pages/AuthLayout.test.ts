import { createRawSnippet } from "svelte";
import { render, screen } from "@testing-library/svelte";
import { describe, expect, it } from "vitest";
import AuthLayout from "./AuthLayout.svelte";

describe("AuthLayout", () => {
  it("pairs an editorial introduction with a daisyUI task card", () => {
    const children = createRawSnippet(() => ({ render: () => "Form content" }));
    const footer = createRawSnippet(() => ({ render: () => "Footer content" }));
    const { container } = render(AuthLayout, {
      title: "Welcome back",
      subtitle: "Sign in to your account.",
      children,
      footer,
    });

    expect(screen.getByRole("region", { name: "SimpleBank introduction" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Welcome back" })).toBeInTheDocument();
    expect(container.querySelector(".card.card-border")).toBeInTheDocument();
  });
});
