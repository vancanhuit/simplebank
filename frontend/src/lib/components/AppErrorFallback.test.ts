import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import { afterEach, describe, expect, it, vi } from "vitest";
import AppErrorFallback from "./AppErrorFallback.svelte";

describe("AppErrorFallback", () => {
  afterEach(() => cleanup());

  it("shows sanitized recovery actions and invokes them", async () => {
    const reset = vi.fn();
    const reload = vi.fn();
    render(AppErrorFallback, { reset, reload });

    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("We couldn't display SimpleBank.");
    expect(alert).not.toHaveTextContent("database password leaked");

    await fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(reset).toHaveBeenCalledOnce();

    await fireEvent.click(screen.getByRole("button", { name: "Reload page" }));
    expect(reload).toHaveBeenCalledOnce();
  });
});
