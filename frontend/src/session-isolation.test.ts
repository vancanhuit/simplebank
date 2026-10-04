import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import { afterEach, expect, it, vi } from "vitest";
import App from "./App.svelte";
import { auth } from "./lib/stores/auth.svelte";
import { accounts } from "./lib/stores/accounts.svelte";
import { notifications } from "./lib/stores/notifications.svelte";
import { router } from "./lib/router.svelte";

const alice = {
  username: "alice",
  full_name: "Alice",
  email: "alice@example.com",
  is_email_verified: true,
  created_at: "2026-01-01T00:00:00Z",
};
const bob = { ...alice, username: "bob", full_name: "Bob" };
const account = {
  id: "alice-account",
  owner: "alice",
  currency: "USD" as const,
  balance: 10000,
  created_at: alice.created_at,
};
const bobAccount = { ...account, id: "bob-account", owner: "bob", balance: 20000 };
const note = {
  id: "alice-note",
  account_id: account.id,
  transfer_id: "alice-transfer",
  direction: "sent" as const,
  amount: 100,
  currency: "USD" as const,
  balance: 10000,
  read_at: null,
  created_at: alice.created_at,
};
const json = (body: unknown) => new Response(JSON.stringify(body), { status: 200 });

afterEach(() => {
  cleanup();
  notifications.reset();
  accounts.reset();
  auth.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("real App resets populated principal state and discards late transfer/account work while same-user renewal retains the form", async () => {
  auth.clear();
  auth.initializing = false;
  auth.user = alice;
  auth.accessToken = "alice-token";
  accounts.loaded = true;
  accounts.items = [account];
  history.replaceState({}, "", "/transfer");
  router.path = "/transfer";
  vi.spyOn(auth, "init").mockResolvedValue();
  let replacement = false;
  let completeTransfer!: (response: Response) => void;
  let completeCreate!: (response: Response) => void;
  let completeLoad!: (response: Response) => void;
  let completeHistory!: (response: Response) => void;
  let holdLoad = false;
  let holdHistory = false;
  const fetchMock = vi.fn((path: string, init?: RequestInit) => {
    if (path.endsWith("/tokens/renew"))
      return Promise.resolve(
        json({
          user: replacement ? bob : alice,
          access_token: replacement ? "bob-token" : "alice-renewed",
          access_token_expires_at: "2026-12-01T00:00:00Z",
        }),
      );
    if (path.endsWith("/notifications/stream")) return new Promise<Response>(() => {});
    if (path.includes("/notifications?")) {
      if (holdHistory) {
        holdHistory = false;
        return new Promise<Response>((resolve) => {
          completeHistory = resolve;
        });
      }
      return Promise.resolve(
        json({
          notifications: replacement ? [] : [note],
          unread_count: replacement ? 0 : 1,
          next_cursor: null,
        }),
      );
    }
    if (path.includes("/accounts?")) {
      if (holdLoad) {
        holdLoad = false;
        return new Promise<Response>((resolve) => {
          completeLoad = resolve;
        });
      }
      return Promise.resolve(json([replacement ? bobAccount : account]));
    }
    if (path.endsWith("/accounts") && init?.method === "POST")
      return new Promise<Response>((resolve) => {
        completeCreate = resolve;
      });
    if (path.endsWith("/transfers"))
      return new Promise<Response>((resolve) => {
        completeTransfer = resolve;
      });
    return Promise.resolve(json({}));
  });
  vi.stubGlobal("fetch", fetchMock);
  render(App);
  const amount = await screen.findByRole("textbox", { name: "Amount (USD)" });
  await waitFor(() => expect(notifications.items).toHaveLength(1));
  await fireEvent.input(amount, { target: { value: "12.30" } });
  await fireEvent.input(screen.getByRole("textbox", { name: "Recipient account id" }), {
    target: { value: "recipient" },
  });
  const generation = auth.generation;
  await auth.tryRefresh();
  expect(auth.generation).toBe(generation);
  expect(screen.getByRole("textbox", { name: "Amount (USD)" })).toBe(amount);
  expect(amount).toHaveValue("12.30");
  await fireEvent.click(screen.getByRole("button", { name: "Send transfer" }));
  holdLoad = true;
  const oldLoad = accounts.load();
  const oldCreate = accounts.create("EUR", 1).catch(() => undefined);
  holdHistory = true;
  const oldHistory = notifications.reconcile("live");
  notifications.toasts = [{ id: note.id, notification: note }];
  accounts.transferFromId = account.id;
  replacement = true;
  await auth.tryRefresh();
  await waitFor(() =>
    expect(screen.getByRole("combobox", { name: "From account" })).toHaveValue("bob-account"),
  );
  expect(notifications.items).toEqual([]);
  expect(notifications.toasts).toEqual([]);
  expect(accounts.transferFromId).toBeNull();
  expect(screen.getByRole("textbox", { name: "Amount (USD)" })).not.toBe(amount);
  expect(screen.getByRole("textbox", { name: "Recipient account id" })).toHaveValue("");
  const calls = fetchMock.mock.calls.length;
  completeLoad(json([account]));
  completeCreate(json({ ...account, id: "late-created" }));
  completeHistory(
    json({ notifications: [{ ...note, id: "late-note" }], unread_count: 99, next_cursor: "old" }),
  );
  completeTransfer(
    json({
      transfer: {
        id: "late-transfer",
        from_account_id: account.id,
        to_account_id: "recipient",
        amount: 1230,
        idempotency_key: "key",
        created_at: alice.created_at,
      },
      from_account: account,
    }),
  );
  await Promise.all([oldLoad, oldCreate, oldHistory]);
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(fetchMock).toHaveBeenCalledTimes(calls);
  expect(accounts.items).toEqual([bobAccount]);
  expect(auth.user?.username).toBe("bob");
  expect(notifications.items).toEqual([]);
  expect(notifications.toasts).toEqual([]);
  expect(screen.queryByText(/Sent .*successfully/)).not.toBeInTheDocument();
});
