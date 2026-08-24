import { beforeEach, describe, expect, it } from "vitest";
import {
  navigate,
  replaceNavigation,
  replaceNavigationState,
  router,
  safeReturnPath,
} from "./router.svelte";

describe("router", () => {
  beforeEach(() => {
    history.replaceState({}, "", "/");
    router.path = "/";
    router.state = {};
  });

  it("normalizes trailing slashes and stores navigation state", () => {
    navigate("/transfer/", { source: "test" });
    expect(router.path).toBe("/transfer");
    expect(router.state).toEqual({ source: "test" });
    expect(location.pathname).toBe("/transfer/");
    expect(history.state).toEqual({ source: "test" });
  });

  it("writes the complete URL while matching only its normalized pathname", () => {
    navigate("/accounts/abc?tab=activity#latest");

    expect(`${location.pathname}${location.search}${location.hash}`).toBe(
      "/accounts/abc?tab=activity#latest",
    );
    expect(router.path).toBe("/accounts/abc");
  });

  it.each([
    ["/transfer", "/transfer"],
    ["/accounts/abc?tab=activity", "/accounts/abc?tab=activity"],
    ["/accounts/abc?tab=activity#latest", "/accounts/abc?tab=activity#latest"],
  ])("accepts safe same-origin return path %s", (value, expected) => {
    expect(safeReturnPath(value)).toBe(expected);
  });

  it.each([
    "https://evil.example",
    "//evil.example",
    "/login",
    "/login?returnTo=/transfer",
    "/register",
    "/transfer\n/hidden",
    "/\\[::1",
    42,
  ])("rejects unsafe return path %s", (value) => {
    expect(safeReturnPath(value)).toBeNull();
  });

  it("tracks browser history navigation", () => {
    history.replaceState({ source: "history" }, "", "/accounts/new/");
    window.dispatchEvent(new PopStateEvent("popstate"));
    expect(router.path).toBe("/accounts/new");
    expect(router.state).toEqual({ source: "history" });
  });

  it("replaces the current navigation state and reactive snapshot together", () => {
    replaceNavigationState({ registered: true });

    expect(history.state).toEqual({ registered: true });
    expect(router.state).toEqual({ registered: true });
  });

  it("replaces the current path and state without adding a history entry", () => {
    navigate("/transfer", { source: "account-card" });
    const historyLength = history.length;
    replaceNavigation("/login/", { logoutFailed: true });

    expect(history.length).toBe(historyLength);
    expect(location.pathname).toBe("/login/");
    expect(history.state).toEqual({ logoutFailed: true });
    expect(router.path).toBe("/login");
    expect(router.state).toEqual({ logoutFailed: true });
  });
});
