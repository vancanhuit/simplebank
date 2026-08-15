import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import TransferPage from "./TransferPage.svelte";
import { accounts } from "../stores/accounts.svelte";

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("TransferPage", () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    fetchMock.mockClear();

    accounts.loaded = true;
    accounts.items = [
      {
        id: "acct-1",
        owner: "alice",
        currency: "USD",
        balance: 100000,
        created_at: "2026-01-01T00:00:00Z",
      },
    ];

    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    accounts.reset();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    cleanup();
  });

  it("consolidates success message and details into one role=status receipt", async () => {
    // Mock transfer limits
    fetchMock.mockResolvedValueOnce(
      jsonResponse(200, {
        USD: { max_per_transfer: 1000000 },
      }),
    );

    render(TransferPage);

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/api/v1/transfer-limits", expect.any(Object));
    });

    // Mock transfer success response
    fetchMock.mockResolvedValueOnce(
      jsonResponse(200, {
        transfer: {
          id: "tx-abc123",
          from_account_id: "acct-1",
          to_account_id: "acct-2",
          amount: 5000,
          currency: "USD",
          created_at: "2026-08-15T12:00:00Z",
        },
        from_account: {
          id: "acct-1",
          owner: "alice",
          currency: "USD",
          balance: 95000,
          created_at: "2026-01-01T00:00:00Z",
        },
      }),
    );

    // Mock accounts reload after transfer
    fetchMock.mockResolvedValueOnce(
      jsonResponse(200, [
        {
          id: "acct-1",
          owner: "alice",
          currency: "USD",
          balance: 95000,
          created_at: "2026-01-01T00:00:00Z",
        },
      ]),
    );

    const toField = screen.getByRole("textbox", { name: /recipient account id/i });
    const amountField = screen.getByRole("spinbutton", { name: /amount/i });
    const submitButton = screen.getByRole("button", { name: /send transfer/i });

    await fireEvent.input(toField, { target: { value: "acct-2" } });
    await fireEvent.input(amountField, { target: { value: "50.00" } });
    await fireEvent.click(submitButton);

    await waitFor(() => {
      const receipt = screen.getByRole("status");
      expect(receipt).toBeInTheDocument();
      expect(receipt).toHaveTextContent("Sent $50.00 successfully");
      expect(receipt).toHaveTextContent("From");
      expect(receipt).toHaveTextContent("$950.00 left");
      expect(receipt).toHaveTextContent("Reference");
      expect(receipt).toHaveTextContent("tx-abc123");

      // Verify definition list structure inside receipt
      const definitionList = receipt.querySelector("dl");
      expect(definitionList).toBeInTheDocument();
    });

    // Verify no separate success Alert exists
    const alerts = screen.queryAllByRole("alert");
    expect(alerts).toHaveLength(0);
  });
});
