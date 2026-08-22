import { afterEach, describe, expect, it } from "vitest";
import { createRawSnippet } from "svelte";
import { cleanup, render, screen } from "@testing-library/svelte";
import Alert from "./Alert.svelte";

describe("Alert", () => {
  afterEach(() => cleanup());

  it.each([
    { variant: "error", role: "alert" as const },
    { variant: "success", role: "status" as const },
    { variant: "info", role: "status" as const },
  ])("asserts static fallback-class for $variant variant", ({ variant, role }) => {
    const children = createRawSnippet(() => ({ render: () => `${variant} message` }));
    render(Alert, { variant: variant as "error" | "success" | "info", children });

    const alert = screen.getByRole(role);

    expect(alert).toHaveClass("alert");
    expect(alert).toHaveClass(
      variant === "error" ? "alert-error" : variant === "success" ? "alert-success" : "alert-info",
    );
    // Alert should have forced-colors fallback for border, background, and text
    expect(alert.className).toContain("forced-colors:border-[CanvasText]");
    expect(alert.className).toContain("forced-colors:bg-[Canvas]");
    expect(alert.className).toContain("forced-colors:text-[CanvasText]");
  });

  it("applies role=alert for error variant and role=status for info/success", () => {
    const children = createRawSnippet(() => ({ render: () => "Test message" }));

    render(Alert, { variant: "error", children });
    expect(screen.getByRole("alert")).toBeInTheDocument();
    cleanup();

    render(Alert, { variant: "info", children });
    expect(screen.getByRole("status")).toBeInTheDocument();
    cleanup();

    render(Alert, { variant: "success", children });
    expect(screen.getByRole("status")).toBeInTheDocument();
  });
});
