<script lang="ts">
  import { onMount } from "svelte";
  import { auth } from "../stores/auth.svelte";
  import { accounts } from "../stores/accounts.svelte";
  import { formatMoney, type Currency } from "../money";
  import AccountCard from "../components/AccountCard.svelte";
  import Alert from "../components/Alert.svelte";
  import Link from "../components/Link.svelte";
  import Send from "@lucide/svelte/icons/send";
  import Plus from "@lucide/svelte/icons/plus";

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

<section class="card bg-neutral text-neutral-content shadow-sm" aria-labelledby="summary-heading">
  <div class="card-body gap-7 p-6 sm:p-8">
    <div class="flex flex-col gap-5 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <span class="badge badge-outline border-neutral-content/30 text-neutral-content">
          Your finances
        </span>
        <h1
          id="summary-heading"
          class="mt-4 max-w-xl text-3xl font-semibold tracking-tight sm:text-4xl"
        >
          Good to see you, {firstName}.
        </h1>
        <p class="mt-2 text-sm text-neutral-content/60">
          {accounts.items.length}
          {accounts.items.length === 1 ? "account" : "accounts"} ready when you are.
        </p>
      </div>
      <div class="flex w-full flex-col gap-3 sm:w-auto sm:flex-row">
        <Link href="/transfer" class="btn btn-primary min-h-11">
          <Send size={16} aria-hidden="true" />
          Send money
        </Link>
        <Link href="/accounts/new" class="btn btn-outline min-h-11 text-neutral-content">
          <Plus size={16} aria-hidden="true" />
          New account
        </Link>
      </div>
    </div>

    {#if totals.length > 0}
      <dl
        class="stats stats-vertical bg-neutral-content/5 text-neutral-content shadow-none lg:stats-horizontal"
      >
        {#each totals as total (total.currency)}
          <div class="stat">
            <dt class="stat-title text-neutral-content/60">{total.currency} balance</dt>
            <dd
              class="stat-value text-2xl tracking-tight text-neutral-content tabular-nums sm:text-3xl"
            >
              {formatMoney(total.amount, total.currency)}
            </dd>
          </div>
        {/each}
      </dl>
    {/if}
  </div>
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
    <h2 id="accounts-heading" class="text-lg font-semibold">Your accounts</h2>
  </div>

  {#if accounts.loading && !accounts.loaded}
    <div
      class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3"
      aria-busy="true"
      aria-label="Loading accounts"
    >
      {#each [0, 1, 2] as placeholder (placeholder)}
        <div class="skeleton h-52"></div>
      {/each}
    </div>
  {:else if accounts.error}
    <Alert variant="error">
      {accounts.error}
      <button
        type="button"
        class="btn btn-ghost btn-sm ml-2 min-h-11"
        onclick={() => accounts.load()}>Retry</button
      >
    </Alert>
  {:else if accounts.items.length === 0}
    <div class="card border border-dashed border-base-300 bg-base-100 text-center">
      <div class="card-body items-center px-6 py-12">
        <h3 class="card-title text-base">No accounts yet</h3>
        <p class="max-w-sm text-sm text-base-content/60">
          Open your first account to start holding and transferring funds.
        </p>
        <div class="card-actions mt-2">
          <Link href="/accounts/new" class="btn btn-primary min-h-11">Open an account</Link>
        </div>
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
