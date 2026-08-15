import { cleanup, render, waitFor } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App.svelte";
import { navigate, router } from "./lib/router.svelte";

const authMock = vi.hoisted(() => ({
  initializing: false,
  isAuthenticated: false,
  user: null,
  init: vi.fn(),
}));

vi.mock("./lib/stores/auth.svelte", () => ({ auth: authMock }));

describe("App routing", () => {
  beforeEach(() => {
    history.replaceState({}, "", "/login");
    router.path = "/login";
    authMock.init.mockClear();
  });
  afterEach(() => cleanup());

  it("updates title, announces navigation, and focuses main", async () => {
    render(App);
    await waitFor(() => expect(document.title).toBe("Sign in · SimpleBank"));

    navigate("/register");

    await waitFor(() => expect(document.title).toBe("Create account · SimpleBank"));
    await waitFor(() => {
      expect(document.querySelector('[aria-live="polite"]')).toHaveTextContent("Create account");
      expect(document.querySelector("main")).toHaveFocus();
    });
  });
});
