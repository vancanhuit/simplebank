<script lang="ts">
  import AppHeader from "./lib/components/AppHeader.svelte";
  import AccountCard from "./lib/components/AccountCard.svelte";
  import ActivityList from "./lib/components/ActivityList.svelte";
  import { formatMoney } from "./lib/money";
  import type { Account, ActivityItem } from "./lib/types";

  const userName = "Maya Alvarez";

  // Placeholder data stands in for the SimpleBank API until the client is wired
  // up. Shapes mirror the backend Account and Transfer models.
  const accounts: Account[] = [
    { id: "acct_9f2c41ab", owner: "Everyday Checking", balance: 482_35, currency: "USD" },
    { id: "acct_1d77e0b4", owner: "Travel Savings", balance: 12_940_00, currency: "EUR" },
    { id: "acct_5a3b8cd9", owner: "Rent Reserve", balance: 34_500_000, currency: "VND" },
  ];

  const activity: ActivityItem[] = [
    {
      id: "txn_01",
      amount: 1_250_00,
      currency: "USD",
      counterparty: "Payroll — Northwind Co.",
      occurredAt: "2026-07-17T09:12:00Z",
    },
    {
      id: "txn_02",
      amount: -68_40,
      currency: "USD",
      counterparty: "Blue Bottle Coffee",
      occurredAt: "2026-07-16T15:38:00Z",
    },
    {
      id: "txn_03",
      amount: -420_00,
      currency: "EUR",
      counterparty: "Transfer to Travel Savings",
      occurredAt: "2026-07-15T20:05:00Z",
    },
    {
      id: "txn_04",
      amount: 90_00,
      currency: "USD",
      counterparty: "Refund — Rivet Store",
      occurredAt: "2026-07-14T11:47:00Z",
    },
  ];

  // Primary account drives the headline balance in the summary band.
  const primary = accounts[0];
  const currencyCount = new Set(accounts.map((a) => a.currency)).size;
</script>

<div class="flex min-h-screen flex-col">
  <a
    href="#overview"
    class="sr-only rounded-md bg-brand px-4 py-2 text-surface focus:not-sr-only focus:absolute focus:top-3 focus:left-3 focus:z-10"
  >
    Skip to content
  </a>

  <AppHeader {userName} />

  <main id="overview" class="mx-auto w-full max-w-6xl flex-1 px-4 py-8 sm:px-6">
    <section
      class="flex flex-col gap-6 rounded-card bg-brand-strong p-6 text-surface sm:flex-row sm:items-center sm:justify-between sm:p-8"
      aria-labelledby="summary-heading"
    >
      <div>
        <h1 id="summary-heading" class="text-sm font-medium text-brand-soft">
          Available balance · {primary.owner}
        </h1>
        <p class="mt-1 text-4xl font-semibold tracking-tight tabular-nums">
          {formatMoney(primary.balance, primary.currency)}
        </p>
        <p class="mt-2 text-sm text-brand-soft">
          Across {accounts.length} accounts in {currencyCount} currencies
        </p>
      </div>
      <div class="flex gap-3">
        <a
          href="#transfer"
          class="rounded-md bg-surface px-4 py-2.5 text-sm font-semibold text-brand-strong transition-colors hover:bg-brand-soft"
        >
          Send money
        </a>
        <a
          href="#deposit"
          class="rounded-md border border-brand-soft/40 px-4 py-2.5 text-sm font-semibold text-surface transition-colors hover:bg-brand"
        >
          Add funds
        </a>
      </div>
    </section>

    <section id="accounts" class="mt-10" aria-labelledby="accounts-heading">
      <div class="mb-4 flex items-center justify-between">
        <h2 id="accounts-heading" class="text-lg font-semibold text-ink">Your accounts</h2>
        <a href="#accounts" class="text-sm font-medium text-brand hover:text-brand-strong">
          View all
        </a>
      </div>
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {#each accounts as account (account.id)}
          <AccountCard {account} />
        {/each}
      </div>
    </section>

    <section id="activity" class="mt-10" aria-labelledby="activity-heading">
      <div class="mb-4 flex items-center justify-between">
        <h2 id="activity-heading" class="text-lg font-semibold text-ink">Recent activity</h2>
        <a href="#activity" class="text-sm font-medium text-brand hover:text-brand-strong">
          View all
        </a>
      </div>
      <ActivityList items={activity} />
    </section>
  </main>

  <footer class="border-t border-border bg-surface">
    <div
      class="mx-auto flex max-w-6xl flex-col gap-1 px-4 py-6 text-sm text-muted sm:flex-row sm:items-center sm:justify-between sm:px-6"
    >
      <p>© 2026 SimpleBank. Demo interface.</p>
      <p>Balances shown are sample data.</p>
    </div>
  </footer>
</div>
