<script lang="ts">
  import { onMount } from "svelte";
  import { request, toMessage } from "../api/client";
  import type { Account, Transfer } from "../api/types";
  import { accounts } from "../stores/accounts.svelte";
  import { router } from "../router.svelte";
  import { formatMoney, formatSignedMoney } from "../money";
  import Alert from "../components/Alert.svelte";
  import Link from "../components/Link.svelte";

  // The account id is the second path segment of /accounts/:id.
  const accountId = $derived(router.path.split("/")[2] ?? "");

  let account = $state<Account | null>(null);
  let transfers = $state<Transfer[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  onMount(load);

  async function load(): Promise<void> {
    loading = true;
    error = null;
    try {
      const id = accountId;
      // Reuse the cached account when available (arriving from the dashboard),
      // otherwise fetch it so deep links and refreshes still resolve.
      account =
        accounts.get(id) ?? (await request<Account>(`/accounts/${id}`, { authenticated: true }));
      transfers = await request<Transfer[]>(`/accounts/${id}/transfers?page=1&size=50`, {
        authenticated: true,
      });
    } catch (err) {
      error = toMessage(err);
    } finally {
      loading = false;
    }
  }

  // Describe a transfer relative to this account: outgoing transfers are debits
  // (negative), incoming are credits (positive).
  function row(transfer: Transfer) {
    const outgoing = transfer.from_account_id === accountId;
    return {
      outgoing,
      signed: outgoing ? -transfer.amount : transfer.amount,
      counterparty: outgoing ? transfer.to_account_id : transfer.from_account_id,
      when: new Date(transfer.created_at).toLocaleString(undefined, {
        dateStyle: "medium",
        timeStyle: "short",
      }),
    };
  }
</script>

<div class="mx-auto max-w-2xl">
  <Link
    href="/"
    class="inline-flex min-h-11 items-center text-sm font-medium text-brand hover:text-brand-strong"
    >← Back</Link
  >
  <h1 class="mt-4 text-2xl font-semibold text-ink">Account activity</h1>

  {#if account}
    <div class="mt-4 flex flex-wrap items-baseline justify-between gap-2">
      <div>
        <p class="text-sm text-muted">
          <span
            class="mr-2 inline-flex items-center rounded-full bg-brand-soft px-2.5 py-0.5 text-xs font-semibold text-brand-strong"
          >
            {account.currency}
          </span>
          <code class="font-mono text-xs break-all text-ink">{account.id}</code>
        </p>
      </div>
      <p class="text-lg font-semibold tracking-tight text-ink tabular-nums">
        {formatMoney(account.balance, account.currency)}
      </p>
    </div>
  {/if}

  {#if error}
    <div class="mt-6">
      <Alert variant="error">
        {error}
        <button
          type="button"
          class="ml-2 inline-flex min-h-11 items-center underline"
          onclick={load}>Retry</button
        >
      </Alert>
    </div>
  {:else if loading}
    <div class="mt-6 flex flex-col gap-3" aria-busy="true" aria-label="Loading activity">
      {#each [0, 1, 2, 3] as placeholder (placeholder)}
        <div class="h-16 animate-pulse rounded-card border border-border bg-surface-muted"></div>
      {/each}
    </div>
  {:else if transfers.length === 0}
    <div
      class="mt-6 rounded-card border border-dashed border-border bg-surface px-6 py-12 text-center"
    >
      <h2 class="text-sm font-semibold text-ink">No activity yet</h2>
      <p class="mx-auto mt-1 max-w-sm text-sm text-muted">
        Transfers to and from this account will appear here.
      </p>
      <div class="mt-4 flex justify-center">
        <Link
          href="/transfer"
          class="inline-flex min-h-11 items-center justify-center rounded-md bg-brand px-4 py-2.5 text-sm font-semibold text-surface transition-colors hover:bg-brand-strong"
        >
          Send money
        </Link>
      </div>
    </div>
  {:else if account}
    <ul class="mt-6 flex flex-col gap-3">
      {#each transfers as transfer (transfer.id)}
        {@const r = row(transfer)}
        <li
          class="flex items-center justify-between gap-4 rounded-card border border-border bg-surface p-4"
        >
          <div class="min-w-0">
            <p class="text-sm font-semibold text-ink">
              {r.outgoing ? "Sent" : "Received"}
            </p>
            <p class="mt-0.5 truncate text-xs text-muted">
              {r.outgoing ? "To" : "From"}
              <code class="font-mono break-all">{r.counterparty}</code>
            </p>
            <p class="mt-0.5 text-xs text-muted">{r.when}</p>
          </div>
          <p
            class="shrink-0 text-sm font-semibold tabular-nums"
            class:text-negative={r.outgoing}
            class:text-positive={!r.outgoing}
          >
            {formatSignedMoney(r.signed, account.currency)}
          </p>
        </li>
      {/each}
    </ul>
  {/if}
</div>
