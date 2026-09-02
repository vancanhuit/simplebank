import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import { afterEach, describe, expect, it } from "vitest";
import LinkTest from "./LinkTest.svelte";

afterEach(cleanup);

describe("Link", () => {
  it("leaves external-origin navigation to the browser", async () => {
    render(LinkTest, { href: "https://example.com/path" });
    const event = new MouseEvent("click", { bubbles: true, cancelable: true });

    await fireEvent(screen.getByRole("link"), event);

    expect(event.defaultPrevented).toBe(false);
  });
});
