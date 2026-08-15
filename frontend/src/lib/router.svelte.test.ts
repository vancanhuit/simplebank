import { beforeEach, describe, expect, it } from "vitest";
import { navigate, router } from "./router.svelte";

describe("router", () => {
  beforeEach(() => {
    history.replaceState({}, "", "/");
    router.path = "/";
  });

  it("normalizes trailing slashes and stores navigation state", () => {
    navigate("/transfer/", { source: "test" });
    expect(router.path).toBe("/transfer");
    expect(location.pathname).toBe("/transfer");
    expect(history.state).toEqual({ source: "test" });
  });

  it("tracks browser history navigation", () => {
    history.replaceState({}, "", "/accounts/new/");
    window.dispatchEvent(new PopStateEvent("popstate"));
    expect(router.path).toBe("/accounts/new");
  });
});
