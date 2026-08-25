import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import type { Notification } from "../api/types";
import { router } from "../router.svelte";
import { notifications } from "../stores/notifications.svelte";
import NotificationBell from "./NotificationBell.svelte";

const unread: Notification = {
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

const read = { ...unread, id: "22222222-2222-2222-2222-222222222222", read_at: "now" };

function seed(items: Notification[], unreadCount: number) {
  notifications.items = items;
  notifications.unreadCount = unreadCount;
}

afterEach(() => {
  notifications.reset();
  router.path = "/";
  router.state = {};
  vi.restoreAllMocks();
});

describe("NotificationBell", () => {
  it("keeps popover anchor declarations out of inline styles", () => {
    render(NotificationBell);

    const trigger = screen.getByRole("button", { name: "Notifications, 0 unread" });
    const region = screen.getByRole("region", { name: "Recent notifications" });

    expect(trigger).not.toHaveAttribute("style");
    expect(trigger).toHaveClass("[anchor-name:--notification-bell]");
    expect(region).not.toHaveAttribute("style");
    expect(region).toHaveClass("[position-anchor:--notification-bell]");
  });

  it("announces the actual unread count while visually capping the hidden badge", () => {
    seed([unread], 124);
    render(NotificationBell);

    const trigger = screen.getByRole("button", { name: "Notifications, 124 unread" });
    expect(trigger).toHaveClass("min-h-11", "min-w-11");
    expect(screen.getByText("99+")).toHaveAttribute("aria-hidden", "true");
  });

  it("does not mark notifications read when opened", async () => {
    seed([unread], 1);
    const markRead = vi.spyOn(notifications, "markRead");
    const markAllRead = vi.spyOn(notifications, "markAllRead");
    render(NotificationBell);

    await fireEvent.click(screen.getByRole("button", { name: "Notifications, 1 unread" }));

    expect(markRead).not.toHaveBeenCalled();
    expect(markAllRead).not.toHaveBeenCalled();
  });

  it("awaits an unread write before closing and navigating", async () => {
    seed([unread], 1);
    let resolve!: () => void;
    const markRead = vi
      .spyOn(notifications, "markRead")
      .mockImplementation(() => new Promise<void>((done) => (resolve = done)));
    const hidePopover = vi.fn();
    render(NotificationBell);
    const popover = screen.getByRole("region", { name: "Recent notifications" });
    Object.defineProperty(popover, "hidePopover", { value: hidePopover });

    await fireEvent.click(screen.getByRole("button", { name: /sent/i }));
    expect(markRead).toHaveBeenCalledWith(unread.id);
    expect(hidePopover).not.toHaveBeenCalled();
    expect(router.path).toBe("/");

    resolve();
    await waitFor(() => expect(router.path).toBe(`/accounts/${unread.account_id}`));
    expect(hidePopover).toHaveBeenCalledOnce();
  });

  it("keeps the popover open and offers retry when marking unread fails", async () => {
    seed([unread], 1);
    const markRead = vi
      .spyOn(notifications, "markRead")
      .mockRejectedValueOnce(new Error("write failed"))
      .mockResolvedValueOnce(undefined);
    const hidePopover = vi.fn();
    render(NotificationBell);
    const popover = screen.getByRole("region", { name: "Recent notifications" });
    Object.defineProperty(popover, "hidePopover", { value: hidePopover });

    await fireEvent.click(screen.getByRole("button", { name: /sent/i }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Something went wrong. Please try again.");
    expect(alert).not.toHaveTextContent("write failed");
    expect(hidePopover).not.toHaveBeenCalled();
    expect(router.path).toBe("/");

    await fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(router.path).toBe(`/accounts/${unread.account_id}`));
    expect(markRead).toHaveBeenCalledTimes(2);
  });

  it("navigates a read row immediately without writing", async () => {
    seed([read], 0);
    const markRead = vi.spyOn(notifications, "markRead");
    const hidePopover = vi.fn();
    render(NotificationBell);
    const popover = screen.getByRole("region", { name: "Recent notifications" });
    Object.defineProperty(popover, "hidePopover", { value: hidePopover });

    await fireEvent.click(screen.getByRole("button", { name: /sent/i }));

    expect(markRead).not.toHaveBeenCalled();
    expect(hidePopover).toHaveBeenCalledOnce();
    expect(router.path).toBe(`/accounts/${read.account_id}`);
  });

  it("exposes native keyboard-operable mark-all and history actions", async () => {
    seed([unread], 1);
    const markAllRead = vi.spyOn(notifications, "markAllRead").mockResolvedValue(undefined);
    const hidePopover = vi.fn();
    render(NotificationBell);
    const popover = screen.getByRole("region", { name: "Recent notifications" });
    Object.defineProperty(popover, "hidePopover", { value: hidePopover });

    const markAll = screen.getByRole("button", { name: "Mark all read" });
    expect(markAll.tagName).toBe("BUTTON");
    await fireEvent.click(markAll);
    expect(markAllRead).toHaveBeenCalledOnce();

    const history = screen.getByRole("link", { name: "View all notifications" });
    expect(history).toHaveAttribute("href", "/notifications");
    await fireEvent.click(history);
    expect(router.path).toBe("/notifications");
    expect(hidePopover).toHaveBeenCalledOnce();
  });
});
