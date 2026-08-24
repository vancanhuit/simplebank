import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import { afterEach, describe, expect, it } from "vitest";
import Root from "./Root.svelte";
import ThrowingComponent, { allowRender } from "./test/ThrowingComponent.svelte";

describe("Root", () => {
  afterEach(() => cleanup());

  it("sanitizes a render failure and retries the failed content", async () => {
    render(Root, { content: ThrowingComponent });

    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("We couldn't display SimpleBank.");
    expect(alert).not.toHaveTextContent("database password leaked");

    allowRender();
    await fireEvent.click(screen.getByRole("button", { name: "Try again" }));

    expect(screen.getByText("Recovered content")).toBeInTheDocument();
  });
});
