import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/svelte";
import type { Notification } from "../api/types";
import NotificationItem from "./NotificationItem.svelte";

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
  read_at: "2026-08-23T10:05:00Z",
};

describe("NotificationItem", () => {
  it("shows sent money as a semantic negative unread row", () => {
    render(NotificationItem, { props: { notification: sent, onactivate: vi.fn() } });

    const row = screen.getByRole("button", { name: /sent.*−.*10\.00.*USD.*2026/i });
    expect(row).toHaveClass("list-row", "min-h-11", "font-semibold");
    expect(screen.getByText(/−.*10\.00/)).toHaveClass("text-error");
    expect(screen.getByText("Unread")).toBeInTheDocument();
  });

  it("shows received money as a semantic positive read row", () => {
    render(NotificationItem, { props: { notification: received, onactivate: vi.fn() } });

    expect(screen.getByText(/\+.*20\.00/)).toHaveClass("text-success");
    expect(screen.queryByText("Unread")).not.toBeInTheDocument();
  });

  it("activates exactly once through its native button", async () => {
    const onactivate = vi.fn();
    render(NotificationItem, { props: { notification: sent, onactivate } });

    await fireEvent.click(screen.getByRole("button", { name: /sent/i }));

    expect(onactivate).toHaveBeenCalledOnce();
    expect(onactivate).toHaveBeenCalledWith(sent);
  });
});
