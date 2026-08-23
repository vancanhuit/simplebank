import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Notification, NotificationPage } from "../api/types";

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
  requestResponse: vi.fn(),
  consumeEventStream: vi.fn(),
  accountsLoad: vi.fn(),
}));

vi.mock("../api/client", () => ({
  request: mocks.request,
  requestResponse: mocks.requestResponse,
  toMessage: (error: unknown) => (error instanceof Error ? error.message : "Request failed"),
}));

vi.mock("../api/sse", () => ({
  consumeEventStream: mocks.consumeEventStream,
}));

vi.mock("./accounts.svelte", () => ({
  accounts: { load: mocks.accountsLoad },
}));

import { notifications } from "./notifications.svelte";
import { auth } from "./auth.svelte";

const sent: Notification = {
  id: "11111111-1111-1111-1111-111111111111",
  account_id: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
  transfer_id: "aaaaaaaa-1111-1111-1111-111111111111",
  direction: "sent",
  amount: 1_000,
  currency: "USD",
  balance: 9_000,
  read_at: null,
  created_at: "2026-08-23T10:00:00Z",
};

const received: Notification = {
  id: "22222222-2222-2222-2222-222222222222",
  account_id: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
  transfer_id: "bbbbbbbb-2222-2222-2222-222222222222",
  direction: "received",
  amount: 2_000,
  currency: "EUR",
  balance: 12_000,
  read_at: null,
  created_at: "2026-08-23T10:01:00Z",
};

const anotherSent: Notification = {
  ...sent,
  id: "33333333-3333-3333-3333-333333333333",
  transfer_id: "aaaaaaaa-3333-3333-3333-333333333333",
  created_at: "2026-08-23T10:02:00Z",
};

function page(
  rows: Notification[],
  unreadCount = rows.filter((row) => row.read_at === null).length,
  nextCursor: string | null = null,
): NotificationPage {
  return { notifications: rows, unread_count: unreadCount, next_cursor: nextCursor };
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

async function flush(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
}

describe("NotificationsStore", () => {
  beforeEach(() => {
    mocks.accountsLoad.mockResolvedValue(true);
    mocks.consumeEventStream.mockImplementation(
      (_response: Response, _onEvent: () => void, signal?: AbortSignal) =>
        new Promise<void>((resolve) =>
          signal?.addEventListener("abort", () => resolve(), { once: true }),
        ),
    );
  });

  afterEach(() => {
    notifications.reset();
    vi.useRealTimers();
    vi.restoreAllMocks();
    vi.clearAllMocks();
  });

  it("loads history and unread count without replaying toasts", async () => {
    mocks.request.mockResolvedValue(page([sent, received], 17, "next"));

    await notifications.reconcile("initial");

    expect(notifications.items).toEqual([sent, received]);
    expect(notifications.unreadCount).toBe(17);
    expect(notifications.nextCursor).toBe("next");
    expect(notifications.toasts).toEqual([]);
  });

  it("coalesces burst invalidations into one reconciliation", async () => {
    const firstPage = deferred<NotificationPage>();
    mocks.request.mockImplementationOnce(() => firstPage.promise).mockResolvedValue(page([sent]));

    const first = notifications.reconcile("live");
    const second = notifications.reconcile("live");
    const third = notifications.reconcile("live");
    firstPage.resolve(page([]));
    await Promise.all([first, second, third]);

    expect(mocks.request).toHaveBeenCalledTimes(2);
  });

  it("refreshes accounts before publishing a live toast", async () => {
    vi.useFakeTimers();
    const accountRefresh = deferred<boolean>();
    mocks.request.mockResolvedValueOnce(page([sent])).mockResolvedValueOnce(page([received, sent]));
    mocks.accountsLoad
      .mockResolvedValueOnce(true)
      .mockImplementationOnce(() => accountRefresh.promise);
    await notifications.reconcile("initial");

    const reconciliation = notifications.reconcile("live");
    await flush();
    expect(notifications.toasts).toEqual([]);

    accountRefresh.resolve(true);
    await reconciliation;
    expect(notifications.toasts.map((toast) => toast.notification.id)).toEqual([received.id]);

    await vi.advanceTimersByTimeAsync(5_000);
    expect(notifications.toasts).toEqual([]);
  });

  it("increments only the account that received new activity", async () => {
    mocks.request
      .mockResolvedValueOnce(page([]))
      .mockResolvedValueOnce(page([received]))
      .mockResolvedValueOnce(page([sent, received]));
    await notifications.reconcile("initial");
    await notifications.reconcile("live");
    expect(notifications.activityVersion(sent.account_id)).toBe(0);
    expect(notifications.activityVersion(received.account_id)).toBe(1);

    await notifications.reconcile("live");
    expect(notifications.activityVersion(sent.account_id)).toBe(1);
    expect(notifications.activityVersion(received.account_id)).toBe(1);
  });

  it("establishes a failed stream baseline without replaying historical toasts", async () => {
    let streamInvalidation!: () => void;
    mocks.request
      .mockRejectedValueOnce(new Error("initial history failed"))
      .mockRejectedValueOnce(new Error("connected history failed"))
      .mockResolvedValueOnce(page([sent]))
      .mockResolvedValueOnce(page([received, sent]));
    mocks.requestResponse.mockResolvedValue(new Response("stream"));
    mocks.consumeEventStream.mockImplementation(
      (_response: Response, onEvent: () => void, signal?: AbortSignal) => {
        streamInvalidation = onEvent;
        return new Promise<void>((resolve) =>
          signal?.addEventListener("abort", () => resolve(), { once: true }),
        );
      },
    );

    notifications.start();
    await vi.waitFor(() => expect(mocks.consumeEventStream).toHaveBeenCalled());
    streamInvalidation();
    await vi.waitFor(() => expect(mocks.request).toHaveBeenCalledTimes(3));

    expect(notifications.items).toEqual([sent]);
    expect(notifications.toasts).toEqual([]);

    streamInvalidation();
    await vi.waitFor(() => expect(mocks.request).toHaveBeenCalledTimes(4));
    expect(notifications.toasts.map((toast) => toast.notification.id)).toEqual([received.id]);
  });

  it("reconnects with bounded backoff and resets delay after connection", async () => {
    vi.useFakeTimers();
    mocks.request.mockResolvedValue(page([]));
    mocks.requestResponse
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce(new Response("stream"))
      .mockRejectedValue(new Error("offline"));
    mocks.consumeEventStream.mockRejectedValue(new Error("disconnected"));

    notifications.start();
    notifications.start();
    await flush();
    expect(mocks.requestResponse).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(500);
    await flush();
    expect(mocks.requestResponse).toHaveBeenCalledTimes(2);

    await vi.advanceTimersByTimeAsync(500);
    await flush();
    expect(mocks.requestResponse).toHaveBeenCalledTimes(3);

    for (const [delay, calls] of [
      [1_000, 4],
      [2_000, 5],
      [5_000, 6],
      [10_000, 7],
      [30_000, 8],
      [30_000, 9],
    ] as const) {
      await vi.advanceTimersByTimeAsync(delay);
      await flush();
      expect(mocks.requestResponse).toHaveBeenCalledTimes(calls);
    }
  });

  it("reconciles visibility recovery without historical toasts", async () => {
    let visibility: DocumentVisibilityState = "hidden";
    vi.spyOn(document, "visibilityState", "get").mockImplementation(() => visibility);
    mocks.request.mockResolvedValueOnce(page([])).mockResolvedValueOnce(page([]));
    mocks.requestResponse.mockResolvedValue(new Response("stream"));
    notifications.start();
    await flush();

    mocks.request.mockResolvedValueOnce(page([sent]));
    visibility = "visible";
    document.dispatchEvent(new Event("visibilitychange"));
    await flush();

    expect(notifications.items).toEqual([sent]);
    expect(notifications.toasts).toEqual([]);
  });

  it("aborts stream and ignores stale results after reset", async () => {
    const history = deferred<NotificationPage>();
    const nextSessionHistory = deferred<NotificationPage>();
    let requestSignal: AbortSignal | undefined;
    let streamSignal: AbortSignal | undefined;
    mocks.request
      .mockImplementationOnce((_path: string, options: { signal?: AbortSignal }) => {
        requestSignal = options.signal;
        return history.promise;
      })
      .mockImplementationOnce(() => nextSessionHistory.promise)
      .mockResolvedValueOnce(page([received]));
    mocks.requestResponse.mockImplementation((_path: string, options: { signal?: AbortSignal }) => {
      streamSignal = options.signal;
      return Promise.resolve(new Response("stream"));
    });

    notifications.start();
    await flush();
    notifications.reset();
    const nextSession = notifications.reconcile("initial");
    const queued = notifications.reconcile("live");
    history.resolve(page([sent], 1));
    await flush();
    nextSessionHistory.resolve(page([]));
    await Promise.all([nextSession, queued]);

    expect(requestSignal?.aborted).toBe(true);
    expect(streamSignal?.aborted).toBe(true);
    expect(mocks.request).toHaveBeenCalledTimes(3);
    expect(notifications.items).toEqual([received]);
    expect(notifications.unreadCount).toBe(1);
  });

  it("merges cursor pages without duplicate notification ids", async () => {
    mocks.request
      .mockResolvedValueOnce(page([sent], 1, "cursor value"))
      .mockResolvedValueOnce(page([sent, received], 2))
      .mockResolvedValueOnce(page([anotherSent], 1));
    await notifications.reconcile("initial");

    await notifications.loadMore();

    expect(mocks.request).toHaveBeenLastCalledWith(
      "/notifications?size=20&cursor=cursor%20value",
      expect.objectContaining({ authenticated: true }),
    );
    expect(notifications.items.map((row) => row.id)).toEqual([sent.id, received.id]);

    await notifications.reconcile("manual");
    expect(notifications.items.map((row) => row.id)).toEqual([
      anotherSent.id,
      sent.id,
      received.id,
    ]);
    expect(notifications.unreadCount).toBe(1);
  });

  it("rolls back mark-read state and reconciles after failure", async () => {
    mocks.request
      .mockResolvedValueOnce(page([sent], 1))
      .mockRejectedValueOnce(new Error("write failed"))
      .mockResolvedValueOnce(page([sent], 1));
    await notifications.reconcile("initial");

    await expect(notifications.markRead(sent.id)).rejects.toThrow("write failed");

    expect(notifications.items[0].read_at).toBeNull();
    expect(notifications.unreadCount).toBe(1);
    expect(notifications.error).toBe("write failed");
    expect(mocks.request).toHaveBeenLastCalledWith(
      "/notifications?size=20",
      expect.objectContaining({ authenticated: true }),
    );
    expect(mocks.accountsLoad).toHaveBeenCalledTimes(2);
  });

  it("uses mutation responses as the authoritative unread count", async () => {
    mocks.request
      .mockResolvedValueOnce(page([sent, received], 2))
      .mockResolvedValueOnce({ unread_count: 41 })
      .mockResolvedValueOnce({ unread_count: 7 });
    await notifications.reconcile("initial");

    await notifications.markRead(sent.id);
    expect(notifications.unreadCount).toBe(41);

    await notifications.markAllRead();
    expect(notifications.unreadCount).toBe(7);
  });

  it("discards a first-page response that crosses a successful mark-read", async () => {
    const staleReconciliation = deferred<NotificationPage>();
    const readSent = { ...sent, read_at: "2026-08-23T11:00:00Z" };
    mocks.request
      .mockResolvedValueOnce(page([sent], 1))
      .mockImplementationOnce(() => staleReconciliation.promise)
      .mockResolvedValueOnce({ unread_count: 0 })
      .mockResolvedValueOnce(page([readSent], 0));
    await notifications.reconcile("initial");

    const reconciliation = notifications.reconcile("manual");
    await flush();
    await notifications.markRead(sent.id);
    staleReconciliation.resolve(page([sent], 1));
    await reconciliation;

    expect(mocks.request).toHaveBeenCalledTimes(4);
    expect(notifications.items).toEqual([readSent]);
    expect(notifications.unreadCount).toBe(0);
  });

  it("discards a pagination response that crosses a successful mark-all", async () => {
    const stalePage = deferred<NotificationPage>();
    const readSent = { ...sent, read_at: "2026-08-23T11:00:00Z" };
    mocks.request
      .mockResolvedValueOnce(page([sent], 2, "next"))
      .mockImplementationOnce(() => stalePage.promise)
      .mockResolvedValueOnce({ unread_count: 0 })
      .mockResolvedValueOnce(page([readSent], 0));
    await notifications.reconcile("initial");

    const pagination = notifications.loadMore();
    await flush();
    await notifications.markAllRead();
    stalePage.resolve(page([received], 2));
    await pagination;
    await vi.waitFor(() => expect(mocks.request).toHaveBeenCalledTimes(4));

    expect(notifications.items).toEqual([readSent]);
    expect(notifications.unreadCount).toBe(0);
    expect(notifications.nextCursor).toBeNull();
  });

  it("marks unloaded unread history when loaded rows are already read", async () => {
    const readSent = { ...sent, read_at: "2026-08-23T11:00:00Z" };
    mocks.request.mockImplementation((path: string) => {
      if (path === "/notifications?size=20") {
        return Promise.resolve(page([readSent], 4));
      }
      if (path === "/notifications/read-all") {
        return Promise.resolve({ unread_count: 0 });
      }
      throw new Error(`unexpected request: ${path}`);
    });
    await notifications.reconcile("initial");

    await notifications.markAllRead();

    expect(mocks.request).toHaveBeenLastCalledWith(
      "/notifications/read-all",
      expect.objectContaining({ method: "PUT", authenticated: true }),
    );
    expect(notifications.items).toEqual([readSent]);
    expect(notifications.unreadCount).toBe(0);
  });

  it("serializes mutations and limits mark-all to its captured rows", async () => {
    const firstMutation = deferred<{ unread_count: number }>();
    const secondMutation = deferred<{ unread_count: number }>();
    mocks.request.mockImplementation((path: string) => {
      if (path === "/notifications?size=20") {
        return Promise.resolve(
          mocks.request.mock.calls.filter(([calledPath]) => calledPath === path).length === 1
            ? page([sent, received], 2)
            : page([anotherSent, sent, received], 3),
        );
      }
      if (path === `/notifications/${sent.id}/read`) {
        return firstMutation.promise;
      }
      if (path === "/notifications/read-all") {
        return secondMutation.promise;
      }
      throw new Error(`unexpected request: ${path}`);
    });
    await notifications.reconcile("initial");

    const markOne = notifications.markRead(sent.id);
    const markAll = notifications.markAllRead();
    await flush();
    expect(
      mocks.request.mock.calls.filter(
        ([path]) => path === `/notifications/${sent.id}/read` || path === "/notifications/read-all",
      ),
    ).toHaveLength(1);

    firstMutation.resolve({ unread_count: 2 });
    await markOne;
    await flush();
    expect(mocks.request).toHaveBeenCalledWith(
      "/notifications/read-all",
      expect.objectContaining({ method: "PUT" }),
    );

    await notifications.reconcile("live");
    secondMutation.resolve({ unread_count: 1 });
    await markAll;

    expect(notifications.unreadCount).toBe(1);
    expect(notifications.items.find((item) => item.id === sent.id)?.read_at).not.toBeNull();
    expect(notifications.items.find((item) => item.id === received.id)?.read_at).not.toBeNull();
    expect(notifications.items.find((item) => item.id === anotherSent.id)?.read_at).toBeNull();
  });

  it("rejects stale session callbacks after the auth generation changes", async () => {
    let streamInvalidation!: () => void;
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
    mocks.request.mockResolvedValue(page([]));
    mocks.requestResponse.mockResolvedValue(new Response("stream"));
    mocks.consumeEventStream.mockImplementation(
      (_response: Response, onEvent: () => void, signal?: AbortSignal) => {
        streamInvalidation = onEvent;
        return new Promise<void>((resolve) =>
          signal?.addEventListener("abort", () => resolve(), { once: true }),
        );
      },
    );
    notifications.start();
    await vi.waitFor(() => expect(mocks.consumeEventStream).toHaveBeenCalled());
    mocks.request.mockClear();

    auth.clear();
    streamInvalidation();
    document.dispatchEvent(new Event("visibilitychange"));
    await flush();

    expect(mocks.request).not.toHaveBeenCalled();
    expect(notifications.items).toEqual([]);
  });

  it("increments activity only for affected account ids", async () => {
    mocks.request
      .mockResolvedValueOnce(page([sent]))
      .mockResolvedValueOnce(page([received, sent]))
      .mockResolvedValueOnce(page([anotherSent, received, sent]));
    await notifications.reconcile("initial");
    expect(notifications.activityVersion(sent.account_id)).toBe(0);

    await notifications.reconcile("connected");
    expect(notifications.activityVersion(received.account_id)).toBe(1);
    expect(notifications.activityVersion(sent.account_id)).toBe(0);
    expect(notifications.toasts).toEqual([]);

    await notifications.reconcile("live");
    expect(notifications.activityVersion(sent.account_id)).toBe(1);
    expect(notifications.activityVersion("cccccccc-cccc-cccc-cccc-cccccccccccc")).toBe(0);
  });
});
