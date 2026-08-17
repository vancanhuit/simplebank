import { cleanup, render, waitFor } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App.svelte";
import { navigate, router } from "./lib/router.svelte";
import { auth } from "./lib/stores/auth.svelte";

const accountsMock = vi.hoisted(() => ({ reset: vi.fn() }));

vi.mock("./lib/stores/accounts.svelte", () => ({ accounts: accountsMock }));

describe("App routing", () => {
  beforeEach(() => {
    history.replaceState({}, "", "/login");
    router.path = "/login";
    auth.clear();
    auth.initializing = false;
    vi.spyOn(auth, "init").mockResolvedValue();
    accountsMock.reset.mockClear();
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

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

  it("clears the account cache when auth resolves signed out", async () => {
    auth.initializing = true;
    render(App);
    expect(accountsMock.reset).not.toHaveBeenCalled();

    auth.initializing = false;

    await waitFor(() => expect(accountsMock.reset).toHaveBeenCalledOnce());
  });
});
