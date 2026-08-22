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

type FetchCall = [input: RequestInfo | URL, init?: RequestInit];

function jsonRequestBody(call: FetchCall): Record<string, unknown> {
  const body = call[1]?.body;
  if (typeof body !== "string") {
    throw new TypeError("expected a JSON request body");
  }
  const parsed: unknown = JSON.parse(body);
  if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new TypeError("expected a JSON object request body");
  }
  return parsed as Record<string, unknown>;
}

describe("TransferPage", () => {
  const fetchMock = vi.fn<(...args: FetchCall) => Promise<Response>>();

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

  it("presents the source account as a daisyUI select", () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(200, {}));

    render(TransferPage);

    const source = screen.getByRole("combobox", { name: "From account" });
    expect(source).toHaveClass("select", "w-full");
  });

  it("gates transfers on account inventory and initializes the requested source after retry", async () => {
    accounts.loaded = false;
    accounts.items = [];
    accounts.transferFromId = "acct-eur";
    let resolveInitialLoad!: () => void;
    const initialLoad = new Promise<void>((resolve) => {
      resolveInitialLoad = resolve;
    });
    const loadSpy = vi
      .spyOn(accounts, "load")
      .mockReturnValueOnce(initialLoad)
      .mockImplementationOnce(() => {
        accounts.error = null;
        accounts.items = [
          {
            id: "acct-usd",
            owner: "alice",
            currency: "USD",
            balance: 100000,
            created_at: "2026-01-01T00:00:00Z",
          },
          {
            id: "acct-eur",
            owner: "alice",
            currency: "EUR",
            balance: 200000,
            created_at: "2026-01-01T00:00:00Z",
          },
        ];
        accounts.loaded = true;
        return Promise.resolve();
      });
    fetchMock.mockResolvedValueOnce(jsonResponse(200, {}));

    render(TransferPage);

    expect(await screen.findByRole("status")).toHaveTextContent("Loading your accounts");
    expect(screen.queryByRole("combobox", { name: "From account" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Send transfer" })).not.toBeInTheDocument();
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/api/v1/transfer-limits", expect.any(Object));
    });

    accounts.error = "offline";
    resolveInitialLoad();

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Couldn't load your accounts. offline",
    );
    expect(screen.queryByRole("combobox", { name: "From account" })).not.toBeInTheDocument();
    await fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    const source = await screen.findByRole("combobox", { name: "From account" });
    expect(source).toHaveValue("acct-eur");
    expect(accounts.transferFromId).toBeNull();
    expect(loadSpy).toHaveBeenCalledTimes(2);
  });

  it("keeps stale transfer controls gated while a retry reload is pending", async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(200, {}));
    render(TransferPage);
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/api/v1/transfer-limits", expect.any(Object));
    });

    accounts.error = "offline";
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Couldn't load your accounts. offline",
    );

    let resolveReload!: () => void;
    const reload = new Promise<void>((resolve) => {
      resolveReload = resolve;
    });
    vi.spyOn(accounts, "load").mockImplementationOnce(() => {
      accounts.loading = true;
      accounts.error = null;
      return reload.then(() => {
        accounts.items = [
          {
            id: "acct-fresh",
            owner: "alice",
            currency: "EUR",
            balance: 200000,
            created_at: "2026-01-02T00:00:00Z",
          },
        ];
        accounts.loading = false;
      });
    });

    await fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByRole("status")).toHaveTextContent("Loading your accounts");
    expect(screen.queryByRole("combobox", { name: "From account" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Send transfer" })).not.toBeInTheDocument();

    resolveReload();

    const source = await screen.findByRole("combobox", { name: "From account" });
    expect(source).toHaveValue("acct-fresh");
    expect(screen.getByRole("button", { name: "Send transfer" })).toBeInTheDocument();
  });

  it("rejects a transfer without a source account", async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(200, {}));
    accounts.transferFromId = "stale-account";

    render(TransferPage);
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/api/v1/transfer-limits", expect.any(Object));
    });

    await fireEvent.click(screen.getByRole("button", { name: "Send transfer" }));

    expect(screen.getByRole("alert")).toHaveTextContent("Choose an account to send from.");
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("rejects the source account as the recipient", async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(200, {}));

    render(TransferPage);
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/api/v1/transfer-limits", expect.any(Object));
    });

    const recipient = screen.getByRole("textbox", { name: "Recipient account id" });
    await fireEvent.input(recipient, { target: { value: "acct-1" } });
    await fireEvent.click(screen.getByRole("button", { name: "Send transfer" }));

    expect(recipient).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("alert")).toHaveTextContent("Choose a different recipient account.");
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("rejects an amount above the per-transfer limit", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse(200, { USD: { max_per_transfer: 10000, daily: 50000 } }),
    );

    render(TransferPage);
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/api/v1/transfer-limits", expect.any(Object));
    });

    await fireEvent.input(screen.getByRole("textbox", { name: "Recipient account id" }), {
      target: { value: "acct-2" },
    });
    const amount = screen.getByRole("spinbutton", { name: "Amount (USD)" });
    await fireEvent.input(amount, { target: { value: "100.01" } });
    await fireEvent.click(screen.getByRole("button", { name: "Send transfer" }));

    expect(amount).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Amount exceeds the $100.00 per-transfer limit.",
    );
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("consolidates success message and details into one role=status receipt", async () => {
    const idempotencyKey = "11111111-1111-4111-8111-111111111111";
    vi.spyOn(crypto, "randomUUID")
      .mockReturnValueOnce(idempotencyKey)
      .mockReturnValue("22222222-2222-4222-8222-222222222222");

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
      expect(receipt).toHaveClass("card");
      expect(receipt).toHaveTextContent("Sent $50.00 successfully");
      expect(receipt).toHaveTextContent("From");
      expect(receipt).toHaveTextContent("$950.00 left");
      expect(receipt).toHaveTextContent("Reference");
      expect(receipt).toHaveTextContent("tx-abc123");

      // Verify definition list structure inside receipt
      const definitionList = receipt.querySelector("dl");
      expect(definitionList).toBeInTheDocument();
      expect(definitionList).toHaveTextContent("Amount");
      expect(definitionList).toHaveTextContent("$50.00");
      expect(definitionList).toHaveTextContent("Remaining balance");
      expect(definitionList).toHaveTextContent("$950.00");
      expect(definitionList).toHaveTextContent("Reference");
      expect(definitionList).toHaveTextContent("tx-abc123");
    });

    expect(fetchMock.mock.calls[1][0]).toBe("/api/v1/transfers");
    expect(jsonRequestBody(fetchMock.mock.calls[1])).toEqual({
      from_account_id: "acct-1",
      to_account_id: "acct-2",
      amount: 5000,
      currency: "USD",
      idempotency_key: idempotencyKey,
    });

    // Verify no separate success Alert exists
    const alerts = screen.queryAllByRole("alert");
    expect(alerts).toHaveLength(0);
  });

  it("reuses the idempotency key when a failed transfer is retried", async () => {
    const idempotencyKey = "11111111-1111-4111-8111-111111111111";
    vi.spyOn(crypto, "randomUUID")
      .mockReturnValueOnce(idempotencyKey)
      .mockReturnValue("22222222-2222-4222-8222-222222222222");
    fetchMock
      .mockResolvedValueOnce(jsonResponse(200, { USD: { max_per_transfer: 1000000 } }))
      .mockResolvedValueOnce(jsonResponse(503, { error: "temporary failure" }))
      .mockResolvedValueOnce(
        jsonResponse(200, {
          transfer: { id: "tx-retry", amount: 5000 },
          from_account: { currency: "USD", balance: 95000 },
        }),
      )
      .mockResolvedValueOnce(jsonResponse(200, accounts.items));

    render(TransferPage);
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/api/v1/transfer-limits", expect.any(Object));
    });

    await fireEvent.input(screen.getByRole("textbox", { name: /recipient account id/i }), {
      target: { value: "acct-2" },
    });
    await fireEvent.input(screen.getByRole("spinbutton", { name: /amount/i }), {
      target: { value: "50.00" },
    });
    const submitButton = screen.getByRole("button", { name: /send transfer/i });
    await fireEvent.click(submitButton);
    await screen.findByText("temporary failure");

    await fireEvent.click(submitButton);
    expect(await screen.findByRole("status")).toHaveTextContent("Sent $50.00 successfully");

    const firstBody = jsonRequestBody(fetchMock.mock.calls[1]);
    const retryBody = jsonRequestBody(fetchMock.mock.calls[2]);
    expect(firstBody.idempotency_key).toBe(idempotencyKey);
    expect(retryBody).toEqual(firstBody);
  });

  it("uses a new idempotency key when the same normalized transfer is submitted after success", async () => {
    vi.spyOn(crypto, "randomUUID")
      .mockReturnValueOnce("11111111-1111-4111-8111-111111111111")
      .mockReturnValueOnce("22222222-2222-4222-8222-222222222222")
      .mockReturnValueOnce("33333333-3333-4333-8333-333333333333");
    fetchMock
      .mockResolvedValueOnce(jsonResponse(200, { USD: { max_per_transfer: 1000000 } }))
      .mockResolvedValueOnce(
        jsonResponse(200, {
          transfer: { id: "tx-first", amount: 5000 },
          from_account: { currency: "USD", balance: 95000 },
        }),
      )
      .mockResolvedValueOnce(jsonResponse(200, accounts.items))
      .mockResolvedValueOnce(
        jsonResponse(200, {
          transfer: { id: "tx-second", amount: 5000 },
          from_account: { currency: "USD", balance: 90000 },
        }),
      )
      .mockResolvedValueOnce(jsonResponse(200, accounts.items));

    render(TransferPage);
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/api/v1/transfer-limits", expect.any(Object));
    });

    const recipient = screen.getByRole("textbox", { name: /recipient account id/i });
    const amount = screen.getByRole("spinbutton", { name: /amount/i });
    const submit = screen.getByRole("button", { name: /send transfer/i });
    await fireEvent.input(recipient, { target: { value: " acct-2 " } });
    await fireEvent.input(amount, { target: { value: "50.00" } });
    await fireEvent.click(submit);
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: /send transfer/i })).toBeEnabled(),
    );

    await fireEvent.input(screen.getByRole("textbox", { name: /recipient account id/i }), {
      target: { value: "acct-2" },
    });
    await fireEvent.input(screen.getByRole("spinbutton", { name: /amount/i }), {
      target: { value: "50.0" },
    });
    await fireEvent.click(screen.getByRole("button", { name: /send transfer/i }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(5));

    const firstBody = jsonRequestBody(fetchMock.mock.calls[1]);
    const secondBody = jsonRequestBody(fetchMock.mock.calls[3]);
    expect(firstBody).toMatchObject({
      to_account_id: "acct-2",
      amount: 5000,
      idempotency_key: "11111111-1111-4111-8111-111111111111",
    });
    expect(secondBody).toMatchObject({
      to_account_id: "acct-2",
      amount: 5000,
      idempotency_key: "22222222-2222-4222-8222-222222222222",
    });
  });

  it("reuses the idempotency key after normalized-equivalent retry edits", async () => {
    vi.spyOn(crypto, "randomUUID")
      .mockReturnValueOnce("11111111-1111-4111-8111-111111111111")
      .mockReturnValueOnce("22222222-2222-4222-8222-222222222222");
    fetchMock
      .mockResolvedValueOnce(jsonResponse(200, { USD: { max_per_transfer: 1000000 } }))
      .mockResolvedValueOnce(jsonResponse(503, { error: "first failure" }))
      .mockResolvedValueOnce(jsonResponse(503, { error: "retry failure" }));

    render(TransferPage);
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/api/v1/transfer-limits", expect.any(Object));
    });

    const recipient = screen.getByRole("textbox", { name: /recipient account id/i });
    const amount = screen.getByRole("spinbutton", { name: /amount/i });
    const submit = screen.getByRole("button", { name: /send transfer/i });
    await fireEvent.input(recipient, { target: { value: " acct-2 " } });
    await fireEvent.input(amount, { target: { value: "50.00" } });
    await fireEvent.click(submit);
    await screen.findByText("first failure");

    await fireEvent.input(recipient, { target: { value: "acct-2" } });
    await fireEvent.input(amount, { target: { value: "50.0" } });
    await fireEvent.click(submit);
    await screen.findByText("retry failure");

    const firstBody = jsonRequestBody(fetchMock.mock.calls[1]);
    const retryBody = jsonRequestBody(fetchMock.mock.calls[2]);
    expect(retryBody).toEqual(firstBody);
    expect(retryBody.idempotency_key).toBe("11111111-1111-4111-8111-111111111111");
  });

  it.each([
    {
      field: "recipient",
      edit: () =>
        fireEvent.input(screen.getByRole("textbox", { name: /recipient account id/i }), {
          target: { value: "acct-3" },
        }),
      changedField: "to_account_id",
      changedValue: "acct-3",
    },
    {
      field: "minor amount",
      edit: () =>
        fireEvent.input(screen.getByRole("spinbutton", { name: /amount/i }), {
          target: { value: "50.01" },
        }),
      changedField: "amount",
      changedValue: 5001,
    },
  ])(
    "rotates the idempotency key when the $field changes after a failed transfer",
    async ({ edit, changedField, changedValue }) => {
      vi.spyOn(crypto, "randomUUID")
        .mockReturnValueOnce("11111111-1111-4111-8111-111111111111")
        .mockReturnValueOnce("22222222-2222-4222-8222-222222222222");
      fetchMock
        .mockResolvedValueOnce(jsonResponse(200, { USD: { max_per_transfer: 1000000 } }))
        .mockResolvedValueOnce(jsonResponse(503, { error: "first failure" }))
        .mockResolvedValueOnce(jsonResponse(503, { error: "changed failure" }));

      render(TransferPage);
      await waitFor(() => {
        expect(fetchMock).toHaveBeenCalledWith("/api/v1/transfer-limits", expect.any(Object));
      });

      await fireEvent.input(screen.getByRole("textbox", { name: /recipient account id/i }), {
        target: { value: "acct-2" },
      });
      await fireEvent.input(screen.getByRole("spinbutton", { name: /amount/i }), {
        target: { value: "50.00" },
      });
      const submitButton = screen.getByRole("button", { name: /send transfer/i });
      await fireEvent.click(submitButton);
      await screen.findByText("first failure");

      await edit();
      await fireEvent.click(submitButton);
      await screen.findByText("changed failure");

      const firstBody = jsonRequestBody(fetchMock.mock.calls[1]);
      const changedBody = jsonRequestBody(fetchMock.mock.calls[2]);
      expect(changedBody[changedField]).toBe(changedValue);
      expect(changedBody.idempotency_key).not.toBe(firstBody.idempotency_key);
    },
  );

  it("rotates the idempotency key when the source account and currency change", async () => {
    accounts.items = [
      ...accounts.items,
      {
        id: "acct-2",
        owner: "alice",
        currency: "EUR",
        balance: 100000,
        created_at: "2026-01-01T00:00:00Z",
      },
    ];
    vi.spyOn(crypto, "randomUUID")
      .mockReturnValueOnce("11111111-1111-4111-8111-111111111111")
      .mockReturnValueOnce("22222222-2222-4222-8222-222222222222");
    fetchMock
      .mockResolvedValueOnce(
        jsonResponse(200, {
          USD: { max_per_transfer: 1000000 },
          EUR: { max_per_transfer: 1000000 },
        }),
      )
      .mockResolvedValueOnce(jsonResponse(503, { error: "first failure" }))
      .mockResolvedValueOnce(jsonResponse(503, { error: "changed failure" }));

    render(TransferPage);
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/api/v1/transfer-limits", expect.any(Object));
    });

    await fireEvent.input(screen.getByRole("textbox", { name: /recipient account id/i }), {
      target: { value: "acct-3" },
    });
    await fireEvent.input(screen.getByRole("spinbutton", { name: /amount/i }), {
      target: { value: "50.00" },
    });
    const submitButton = screen.getByRole("button", { name: /send transfer/i });
    await fireEvent.click(submitButton);
    await screen.findByText("first failure");

    await fireEvent.change(screen.getByRole("combobox", { name: "From account" }), {
      target: { value: "acct-2" },
    });
    await fireEvent.click(submitButton);
    await screen.findByText("changed failure");

    const firstBody = jsonRequestBody(fetchMock.mock.calls[1]);
    const changedBody = jsonRequestBody(fetchMock.mock.calls[2]);
    expect(firstBody).toMatchObject({ from_account_id: "acct-1", currency: "USD" });
    expect(changedBody).toMatchObject({ from_account_id: "acct-2", currency: "EUR" });
    expect(changedBody.idempotency_key).not.toBe(firstBody.idempotency_key);
  });

  it("does not rotate the idempotency key for invalid local submissions", async () => {
    const randomUUID = vi
      .spyOn(crypto, "randomUUID")
      .mockReturnValueOnce("11111111-1111-4111-8111-111111111111");
    fetchMock
      .mockResolvedValueOnce(jsonResponse(200, { USD: { max_per_transfer: 1000000 } }))
      .mockResolvedValueOnce(jsonResponse(503, { error: "temporary failure" }));

    render(TransferPage);
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/api/v1/transfer-limits", expect.any(Object));
    });

    const submitButton = screen.getByRole("button", { name: /send transfer/i });
    await fireEvent.click(submitButton);
    expect(await screen.findByText("Enter the recipient account id.")).toBeInTheDocument();
    expect(randomUUID).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await fireEvent.input(screen.getByRole("textbox", { name: /recipient account id/i }), {
      target: { value: "acct-2" },
    });
    await fireEvent.input(screen.getByRole("spinbutton", { name: /amount/i }), {
      target: { value: "50.00" },
    });
    await fireEvent.click(submitButton);
    await screen.findByText("temporary failure");

    expect(randomUUID).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("clears recipient validation when the user edits the field", async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(200, {}));

    render(TransferPage);

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/api/v1/transfer-limits", expect.any(Object));
    });

    await fireEvent.click(screen.getByRole("button", { name: "Send transfer" }));

    const recipient = screen.getByRole("textbox", { name: "Recipient account id" });
    expect(recipient).toHaveFocus();
    expect(recipient).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("alert")).toHaveTextContent("Enter the recipient account id.");

    await fireEvent.input(recipient, { target: { value: "acct-2" } });

    expect(recipient).not.toHaveAttribute("aria-invalid");
    expect(screen.queryByText("Enter the recipient account id.")).not.toBeInTheDocument();
  });

  it("clears amount validation when the user edits the field", async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(200, {}));

    render(TransferPage);

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/api/v1/transfer-limits", expect.any(Object));
    });

    await fireEvent.input(screen.getByRole("textbox", { name: "Recipient account id" }), {
      target: { value: "acct-2" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Send transfer" }));

    const amount = screen.getByRole("spinbutton", { name: "Amount (USD)" });
    expect(amount).toHaveFocus();
    expect(amount).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("alert")).toHaveTextContent("Enter an amount greater than zero.");

    await fireEvent.input(amount, { target: { value: "10.00" } });

    expect(amount).not.toHaveAttribute("aria-invalid");
    expect(screen.queryByText("Enter an amount greater than zero.")).not.toBeInTheDocument();
  });
});
