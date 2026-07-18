import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/svelte";
import AccountCard from "./AccountCard.svelte";
import type { Account } from "../api/types";

const account: Account = {
  id: "11111111-2222-3333-4444-555566667777",
  owner: "alice",
  balance: 48235,
  currency: "USD",
  created_at: "2026-01-15T10:00:00Z",
};

describe("AccountCard", () => {
  it("renders the formatted balance and currency", () => {
    render(AccountCard, { props: { account } });

    expect(screen.getByText(/482\.35/)).toBeInTheDocument();
    expect(screen.getByText("USD")).toBeInTheDocument();
  });

  it("shows the full account number", () => {
    render(AccountCard, { props: { account } });

    expect(screen.getByText("11111111-2222-3333-4444-555566667777")).toBeInTheDocument();
  });

  it("exposes a copy-account-number action", () => {
    render(AccountCard, { props: { account } });

    expect(screen.getByRole("button", { name: /copy account number/i })).toBeInTheDocument();
  });

  it("exposes a send-money action", () => {
    render(AccountCard, { props: { account } });

    expect(screen.getByRole("button", { name: /send money/i })).toBeInTheDocument();
  });
});
