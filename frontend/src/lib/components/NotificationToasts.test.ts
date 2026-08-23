import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import type { Notification } from "../api/types";
import { notifications } from "../stores/notifications.svelte";
import NotificationToasts from "./NotificationToasts.svelte";

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
  ...sent,
  id: "22222222-2222-2222-2222-222222222222",
  direction: "received",
  amount: 2_000,
  currency: "EUR",
};

afterEach(() => notifications.reset());

describe("NotificationToasts", () => {
  it("keeps a polite region, describes both directions, and does not steal focus", () => {
    notifications.toasts = [
      { id: sent.id, notification: sent },
      { id: received.id, notification: received },
    ];
    const input = document.createElement("input");
    document.body.append(input);
    input.focus();

    const { container } = render(NotificationToasts);

    const region = container.querySelector('[aria-live="polite"]');
    expect(region).toHaveClass("toast", "toast-top", "toast-end");
    expect(region).toHaveAttribute("aria-atomic", "false");
    expect(region?.querySelectorAll(".alert")).toHaveLength(2);
    expect(region?.querySelector('[role="alert"]')).not.toBeInTheDocument();
    expect(screen.getByText(/sent.*−.*10\.00/i)).toBeInTheDocument();
    expect(screen.getByText(/received.*\+.*20\.00/i)).toBeInTheDocument();
    expect(input).toHaveFocus();
  });

  it("removes only the expired keyed toast when the store timer fires", async () => {
    vi.useFakeTimers();
    notifications.toasts = [
      { id: sent.id, notification: sent },
      { id: received.id, notification: received },
    ];
    render(NotificationToasts);

    setTimeout(() => notifications.dismissToast(sent.id), 5_000);
    await vi.advanceTimersByTimeAsync(5_000);

    expect(screen.queryByText(/sent.*−.*10\.00/i)).not.toBeInTheDocument();
    expect(screen.getByText(/received.*\+.*20\.00/i)).toBeInTheDocument();
    vi.useRealTimers();
  });
});
