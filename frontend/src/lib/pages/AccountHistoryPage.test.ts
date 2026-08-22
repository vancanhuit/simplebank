import { cleanup, render, screen, waitFor } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Account } from "../api/types";
import { navigate, router } from "../router.svelte";
import { accounts } from "../stores/accounts.svelte";
import AccountHistoryPage from "./AccountHistoryPage.svelte";

const requestMock = vi.hoisted(() => vi.fn());

vi.mock("../api/client", () => ({
  request: requestMock,
  toMessage: () => "request failed",
}));

const accountA = "11111111-1111-1111-1111-111111111111";
const accountB = "22222222-2222-2222-2222-222222222222";

function account(id: string): Account {
  return {
    id,
    owner: "alice",
    balance: 10_000,
    currency: "USD",
    created_at: "2026-01-01T00:00:00Z",
  };
}

describe("AccountHistoryPage", () => {
  beforeEach(() => {
    history.replaceState({}, "", `/accounts/${accountA}`);
    router.path = `/accounts/${accountA}`;
    accounts.reset();
    requestMock.mockReset();
    requestMock.mockImplementation((path: string) => {
      const id = path.includes(accountB) ? accountB : accountA;
      return Promise.resolve(path.includes("/transfers") ? [] : account(id));
    });
  });

  afterEach(() => {
    cleanup();
    accounts.reset();
  });

  it("renders loaded transfers as a daisyUI activity list", async () => {
    requestMock.mockImplementation((path: string) =>
      Promise.resolve(
        path.includes("/transfers")
          ? [
              {
                id: "tx-1",
                from_account_id: accountA,
                to_account_id: accountB,
                amount: 2500,
                created_at: "2026-01-02T00:00:00Z",
              },
            ]
          : account(accountA),
      ),
    );

    const { container } = render(AccountHistoryPage);

    expect(await screen.findByText("Sent")).toBeInTheDocument();
    expect(container.querySelector("ul.list")).toBeInTheDocument();
    expect(container.querySelector("li.list-row")).toBeInTheDocument();
  });

  it("reloads account activity when the route account changes", async () => {
    render(AccountHistoryPage);
    expect(await screen.findByText(accountA)).toBeInTheDocument();

    navigate(`/accounts/${accountB}`);

    expect(await screen.findByText(accountB)).toBeInTheDocument();
    expect(requestMock).toHaveBeenCalledWith(`/accounts/${accountB}`, {
      authenticated: true,
    });
    expect(requestMock).toHaveBeenCalledWith(`/accounts/${accountB}/transfers?page=1&size=50`, {
      authenticated: true,
    });
  });

  it("ignores a stale response from the previous account route", async () => {
    let resolveAccountA: (value: Account) => void;
    requestMock.mockImplementation((path: string) => {
      if (path === `/accounts/${accountA}`) {
        return new Promise<Account>((resolve) => {
          resolveAccountA = resolve;
        });
      }
      if (path === `/accounts/${accountB}`) {
        return Promise.resolve(account(accountB));
      }
      return Promise.resolve([]);
    });

    render(AccountHistoryPage);
    await waitFor(() =>
      expect(requestMock).toHaveBeenCalledWith(`/accounts/${accountA}`, {
        authenticated: true,
      }),
    );

    navigate(`/accounts/${accountB}`);
    expect(await screen.findByText(accountB)).toBeInTheDocument();

    resolveAccountA!(account(accountA));
    await waitFor(() =>
      expect(requestMock).toHaveBeenCalledWith(`/accounts/${accountA}/transfers?page=1&size=50`, {
        authenticated: true,
      }),
    );

    expect(screen.getByText(accountB)).toBeInTheDocument();
    expect(screen.queryByText(accountA)).not.toBeInTheDocument();
  });
});
