import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Notification } from "../api/types";
import { router } from "../router.svelte";
import { notifications } from "../stores/notifications.svelte";
import NotificationsPage from "./NotificationsPage.svelte";

const accountId = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const unread: Notification = {
  id: "11111111-1111-1111-1111-111111111111",
  account_id: accountId,
  transfer_id: "aaaaaaaa-1111-1111-1111-111111111111",
  direction: "sent",
  amount: 1_000,
  currency: "USD",
  balance: 9_000,
  read_at: null,
  created_at: "2026-08-23T10:00:00Z",
};
const read: Notification = {
  ...unread,
  id: "22222222-2222-2222-2222-222222222222",
  direction: "received",
  read_at: "2026-08-23T10:05:00Z",
};

describe("NotificationsPage", () => {
  beforeEach(() => {
    notifications.reset();
    router.path = "/notifications";
    router.state = {};
  });

  afterEach(() => {
    cleanup();
    notifications.reset();
    vi.restoreAllMocks();
  });

  it("renders loading, empty, and retained-data error states", () => {
    notifications.loading = true;
    const { unmount } = render(NotificationsPage);
    expect(screen.getByLabelText("Loading notifications")).toBeInTheDocument();
    unmount();

    notifications.loading = false;
    render(NotificationsPage);
    expect(screen.getByText("No notifications yet")).toBeInTheDocument();
    cleanup();

    notifications.items = [read];
    notifications.error = "Could not refresh notifications";
    render(NotificationsPage);
    expect(screen.getByRole("button", { name: /received/i })).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("Could not refresh notifications");
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
  });

  it("marks an unread item before navigating to its account", async () => {
    notifications.items = [unread];
    notifications.unreadCount = 1;
    let resolve!: () => void;
    const markRead = vi.spyOn(notifications, "markRead").mockImplementation(
      () =>
        new Promise<void>((done) => {
          resolve = done;
        }),
    );
    render(NotificationsPage);

    await fireEvent.click(screen.getByRole("button", { name: /sent/i }));

    expect(markRead).toHaveBeenCalledWith(unread.id);
    expect(router.path).toBe("/notifications");

    resolve();
    await waitFor(() => expect(router.path).toBe(`/accounts/${accountId}`));
  });

  it("keeps the page visible when mark-read fails", async () => {
    notifications.items = [unread];
    notifications.unreadCount = 1;
    vi.spyOn(notifications, "markRead").mockRejectedValue(new Error("write failed"));
    render(NotificationsPage);

    await fireEvent.click(screen.getByRole("button", { name: /sent/i }));

    expect(router.path).toBe("/notifications");
    expect(screen.getByRole("heading", { name: "Notifications" })).toBeInTheDocument();
    expect(await screen.findByRole("alert")).toHaveTextContent("write failed");
  });

  it("loads the next cursor page without duplicate rows", async () => {
    notifications.items = [unread];
    notifications.nextCursor = "next";
    vi.spyOn(notifications, "loadMore").mockImplementation(() => {
      notifications.items = [unread, read];
      notifications.nextCursor = null;
      return Promise.resolve();
    });
    render(NotificationsPage);

    await fireEvent.click(screen.getByRole("button", { name: "Load more" }));

    expect(screen.getAllByRole("button", { name: /sent/i })).toHaveLength(1);
    expect(screen.getAllByRole("button", { name: /received/i })).toHaveLength(1);
    expect(screen.queryByRole("button", { name: "Load more" })).not.toBeInTheDocument();
  });

  it("marks all notifications read", async () => {
    notifications.items = [unread];
    notifications.unreadCount = 1;
    const markAllRead = vi
      .spyOn(notifications, "markAllRead")
      .mockImplementation(() => Promise.resolve());
    render(NotificationsPage);

    await fireEvent.click(screen.getByRole("button", { name: "Mark all read" }));

    expect(markAllRead).toHaveBeenCalledOnce();
  });
});
