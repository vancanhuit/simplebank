import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SvelteMap } from "svelte/reactivity";
import type { Account, Transfer } from "../api/types";
import { navigate, router } from "../router.svelte";
import { accounts } from "../stores/accounts.svelte";
import { auth } from "../stores/auth.svelte";
import AccountHistoryPage from "./AccountHistoryPage.svelte";

const requestMock = vi.hoisted(() => vi.fn());
const notificationsMock = vi.hoisted(() => ({
  versions: new Map<string, import("svelte/reactivity").SvelteMap<string, number>>(),
  activityVersion: vi.fn((id: string) => notificationsMock.versions.get(id)?.get("value") ?? 0),
}));

vi.mock("../api/client", () => ({
  request: requestMock,
  toMessage: () => "request failed",
}));
vi.mock("../stores/notifications.svelte", () => ({ notifications: notificationsMock }));

const accountA = "11111111-1111-1111-1111-111111111111";
const accountB = "22222222-2222-2222-2222-222222222222";

function expectRequest(path: string) {
  const call = requestMock.mock.calls.find(([calledPath]) => calledPath === path);
  expect(call).toBeDefined();
  const options = call?.[1] as { authenticated?: boolean; signal?: AbortSignal } | undefined;
  expect(options?.authenticated).toBe(true);
  expect(options?.signal).toBeInstanceOf(AbortSignal);
}

function bumpActivity(id: string) {
  const version = notificationsMock.versions.get(id);
  version?.set("value", (version.get("value") ?? 0) + 1);
}

function account(id: string): Account {
  return {
    id,
    owner: "alice",
    balance: 10_000,
    currency: "USD",
    created_at: "2026-01-01T00:00:00Z",
  };
}

function transfer(id: string, from = accountA, to = accountB): Transfer {
  return {
    id,
    from_account_id: from,
    to_account_id: to,
    amount: 2_500,
    idempotency_key: `${id}-key`,
    created_at: "2026-01-02T00:00:00Z",
  };
}

describe("AccountHistoryPage", () => {
  beforeEach(() => {
    history.replaceState({}, "", `/accounts/${accountA}`);
    router.path = `/accounts/${accountA}`;
    accounts.reset();
    requestMock.mockReset();
    notificationsMock.versions = new Map([
      [accountA, new SvelteMap([["value", 0]])],
      [accountB, new SvelteMap([["value", 0]])],
    ]);
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
                idempotency_key: "tx-1-key",
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
    expectRequest(`/accounts/${accountB}`);
    expectRequest(`/accounts/${accountB}/transfers?page=1&size=50`);
  });

  it("shows loading when revisiting A after navigating A to B to A", async () => {
    render(AccountHistoryPage);
    expect(await screen.findByText(accountA)).toBeInTheDocument();
    navigate(`/accounts/${accountB}`);
    expect(await screen.findByText(accountB)).toBeInTheDocument();

    requestMock.mockImplementation(() => new Promise(() => undefined));
    navigate(`/accounts/${accountA}`);

    expect(await screen.findByLabelText("Loading activity")).toBeInTheDocument();
    expect(screen.queryByText(accountB)).not.toBeInTheDocument();
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
    await waitFor(() => expectRequest(`/accounts/${accountA}`));

    navigate(`/accounts/${accountB}`);
    expect(await screen.findByText(accountB)).toBeInTheDocument();

    resolveAccountA!(account(accountA));
    await waitFor(() => expectRequest(`/accounts/${accountA}/transfers?page=1&size=50`));

    expect(screen.getByText(accountB)).toBeInTheDocument();
    expect(screen.queryByText(accountA)).not.toBeInTheDocument();
  });

  it("reloads the affected account and transfers when its activity version changes", async () => {
    render(AccountHistoryPage);
    expect(await screen.findByText(accountA)).toBeInTheDocument();
    requestMock.mockClear();

    bumpActivity(accountB);

    await Promise.resolve();
    expect(requestMock).not.toHaveBeenCalled();

    bumpActivity(accountA);

    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(2));
    expectRequest(`/accounts/${accountA}`);
    expectRequest(`/accounts/${accountA}/transfers?page=1&size=50`);
  });

  it("keeps successful data visible while a live refresh is pending", async () => {
    requestMock.mockImplementation((path: string) =>
      Promise.resolve(path.includes("/transfers") ? [transfer("tx-existing")] : account(accountA)),
    );
    render(AccountHistoryPage);
    expect(await screen.findByText("Sent")).toBeInTheDocument();
    let resolveAccount!: (value: Account) => void;
    requestMock.mockImplementation((path: string) =>
      path.includes("/transfers")
        ? Promise.resolve([transfer("tx-refreshed")])
        : new Promise<Account>((resolve) => (resolveAccount = resolve)),
    );

    bumpActivity(accountA);

    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(4));
    expect(screen.getByText(accountA)).toBeInTheDocument();
    expect(screen.getByText("Sent")).toBeInTheDocument();
    expect(screen.queryByLabelText("Loading activity")).not.toBeInTheDocument();
    resolveAccount(account(accountA));
  });

  it("keeps successful data and offers compact retry after refresh failure", async () => {
    requestMock.mockImplementation((path: string) =>
      Promise.resolve(path.includes("/transfers") ? [transfer("tx-existing")] : account(accountA)),
    );
    render(AccountHistoryPage);
    expect(await screen.findByText("Sent")).toBeInTheDocument();
    requestMock.mockRejectedValue(new Error("refresh failed"));

    bumpActivity(accountA);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "We couldn't refresh this account's activity. request failed",
    );
    expect(screen.getByText(accountA)).toBeInTheDocument();
    expect(screen.getByText("Sent")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
  });

  it("shows initial loading again when retrying without successful data", async () => {
    requestMock.mockRejectedValueOnce(new Error("initial failure"));
    render(AccountHistoryPage);
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "We couldn't load this account's activity. request failed",
    );
    const nextAccount = new Promise<Account>(() => undefined);
    const nextTransfers = new Promise<Transfer[]>(() => undefined);
    requestMock
      .mockImplementationOnce(() => nextAccount)
      .mockImplementationOnce(() => nextTransfers);

    await fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(screen.getByLabelText("Loading activity")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("ignores a stale refresh response after the auth generation changes", async () => {
    requestMock.mockImplementation((path: string) =>
      Promise.resolve(path.includes("/transfers") ? [transfer("tx-existing")] : account(accountA)),
    );
    render(AccountHistoryPage);
    expect(await screen.findByText("Sent")).toBeInTheDocument();
    let resolveAccount!: (value: Account) => void;
    requestMock.mockImplementation((path: string) =>
      path.includes("/transfers")
        ? Promise.resolve([transfer("tx-stale", accountB, accountA)])
        : new Promise<Account>((resolve) => (resolveAccount = resolve)),
    );
    bumpActivity(accountA);
    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(4));

    auth.clear();
    resolveAccount({ ...account(accountA), balance: 20_000 });

    await Promise.resolve();
    expect(screen.getByText("$100.00")).toBeInTheDocument();
    expect(screen.getByText("Sent")).toBeInTheDocument();
    expect(screen.queryByText("$200.00")).not.toBeInTheDocument();
  });
});
