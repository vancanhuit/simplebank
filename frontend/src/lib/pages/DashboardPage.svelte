<script lang="ts">
  import { onMount } from "svelte";
  import { auth } from "../stores/auth.svelte";
  import { accounts } from "../stores/accounts.svelte";
  import { formatMoney, type Currency } from "../money";
  import AccountCard from "../components/AccountCard.svelte";
  import Alert from "../components/Alert.svelte";
  import Link from "../components/Link.svelte";

  onMount(() => {
    if (!accounts.loaded) {
      void accounts.load();
    }
  });

  const firstName = $derived(auth.user?.full_name.split(" ")[0] ?? "there");

  // Sum balances per currency so the summary band reflects real holdings
  // instead of conflating currencies into one meaningless total.
  const totals = $derived.by(() => {
    const order: Currency[] = [];
    const sums: Partial<Record<Currency, number>> = {};
    for (const account of accounts.items) {
      if (sums[account.currency] === undefined) {
        order.push(account.currency);
      }
      sums[account.currency] = (sums[account.currency] ?? 0) + account.balance;
    }
    return order.map((currency) => ({ currency, amount: sums[currency] ?? 0 }));
  });
</script>

<section
  class="flex flex-col gap-6 rounded-card bg-brand-strong p-6 text-surface sm:p-8"
  aria-labelledby="summary-heading"
>
  <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
    <div>
      <h1 id="summary-heading" class="text-sm font-medium text-brand-soft">
        Good to see you, {firstName}
      </h1>
      <p class="mt-2 text-sm text-brand-soft">
        {accounts.items.length}
        {accounts.items.length === 1 ? "account" : "accounts"}
      </p>
    </div>
    <div class="flex gap-3">
      <Link
        href="/transfer"
        class="rounded-md bg-surface px-4 py-2.5 text-sm font-semibold text-brand-strong transition-colors hover:bg-brand-soft"
      >
        Send money
      </Link>
      <Link
        href="/accounts/new"
        class="rounded-md border border-brand-soft/40 px-4 py-2.5 text-sm font-semibold text-surface transition-colors hover:bg-brand"
      >
        New account
      </Link>
    </div>
  </div>

  {#if totals.length > 0}
    <dl class="flex flex-wrap gap-x-10 gap-y-4">
      {#each totals as total (total.currency)}
        <div>
          <dt class="text-xs font-medium text-brand-soft">{total.currency} balance</dt>
          <dd class="mt-1 text-3xl font-semibold tracking-tight tabular-nums">
            {formatMoney(total.amount, total.currency)}
          </dd>
        </div>
      {/each}
    </dl>
  {/if}
</section>

{#if auth.user && !auth.user.is_email_verified}
  <div class="mt-6">
    <Alert variant="info">
      Your email isn't verified yet. Check your inbox for the verification link.
    </Alert>
  </div>
{/if}

<section class="mt-10" aria-labelledby="accounts-heading">
  <div class="mb-4 flex items-center justify-between">
    <h2 id="accounts-heading" class="text-lg font-semibold text-ink">Your accounts</h2>
  </div>

  {#if accounts.loading && !accounts.loaded}
    <div
      class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3"
      aria-busy="true"
      aria-label="Loading accounts"
    >
      {#each [0, 1, 2] as placeholder (placeholder)}
        <div class="h-40 animate-pulse rounded-card border border-border bg-surface-muted"></div>
      {/each}
    </div>
  {:else if accounts.error}
    <Alert variant="error">
      {accounts.error}
      <button type="button" class="ml-2 underline" onclick={() => accounts.load()}>Retry</button>
    </Alert>
  {:else if accounts.items.length === 0}
    <div class="rounded-card border border-dashed border-border bg-surface px-6 py-12 text-center">
      <h3 class="text-sm font-semibold text-ink">No accounts yet</h3>
      <p class="mx-auto mt-1 max-w-sm text-sm text-muted">
        Open your first account to start holding and transferring funds.
      </p>
      <div class="mt-4 flex justify-center">
        <Link
          href="/accounts/new"
          class="inline-flex items-center justify-center rounded-md bg-brand px-4 py-2.5 text-sm font-semibold text-surface transition-colors hover:bg-brand-strong"
        >
          Open an account
        </Link>
      </div>
    </div>
  {:else}
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {#each accounts.items as account (account.id)}
        <AccountCard {account} />
      {/each}
    </div>
  {/if}
</section>
