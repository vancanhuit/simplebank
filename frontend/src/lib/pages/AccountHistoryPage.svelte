<script lang="ts">
  import { request, toMessage } from "../api/client";
  import type { Account, Transfer } from "../api/types";
  import { auth } from "../stores/auth.svelte";
  import { notifications } from "../stores/notifications.svelte";
  import { router } from "../router.svelte";
  import { formatMoney, formatSignedMoney } from "../money";
  import Alert from "../components/Alert.svelte";
  import Link from "../components/Link.svelte";
  import ArrowUpRight from "@lucide/svelte/icons/arrow-up-right";
  import ArrowDownLeft from "@lucide/svelte/icons/arrow-down-left";

  // The account id is the second path segment of /accounts/:id.
  const accountId = $derived(router.path.split("/")[2] ?? "");

  let account = $state<Account | null>(null);
  let transfers = $state<Transfer[]>([]);
  let loading = $state(true);
  let refreshing = $state(false);
  let error = $state<string | null>(null);
  let loadGeneration = 0;
  let loadedRouteId: string | null = null;
  let retryVersion = $state(0);

  $effect(() => {
    const id = accountId;
    const activityVersion = notifications.activityVersion(id);
    const authGeneration = auth.generation;
    void activityVersion;
    void retryVersion;
    const preserveVisibleData = loadedRouteId === id;
    loadedRouteId = id;
    const controller = new AbortController();
    void load(id, ++loadGeneration, authGeneration, controller.signal, preserveVisibleData);
    return () => controller.abort();
  });

  async function load(
    id: string,
    generation: number,
    authGeneration: number,
    signal: AbortSignal,
    preserveVisibleData: boolean,
  ): Promise<void> {
    if (preserveVisibleData) {
      refreshing = true;
    } else {
      loading = true;
      account = null;
      transfers = [];
    }
    error = null;
    const current = () =>
      loadGeneration === generation && auth.generation === authGeneration && !signal.aborted;
    try {
      const [nextAccount, nextTransfers] = await Promise.all([
        request<Account>(`/accounts/${id}`, { authenticated: true, signal }),
        request<Transfer[]>(`/accounts/${id}/transfers?page=1&size=50`, {
          authenticated: true,
          signal,
        }),
      ]);
      if (!current()) {
        return;
      }
      account = nextAccount;
      transfers = nextTransfers;
    } catch (err) {
      if (current()) {
        error = toMessage(err);
      }
    } finally {
      if (current()) {
        loading = false;
        refreshing = false;
      }
    }
  }

  function retry() {
    retryVersion += 1;
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

<div class="mx-auto max-w-3xl">
  <Link href="/" class="btn btn-ghost -ml-4 min-h-11">← Back</Link>
  <h1 class="mt-6 text-3xl font-bold tracking-tight text-base-content sm:text-4xl">
    Account activity
  </h1>

  {#if account}
    <div class="card mt-6 border border-base-300 bg-base-100 shadow-sm">
      <div class="card-body gap-4 p-5 sm:flex-row sm:items-center sm:justify-between sm:p-6">
        <div class="min-w-0">
          <span class="badge badge-primary badge-soft">{account.currency}</span>
          <code class="mt-3 block font-mono text-xs break-all text-base-content/70">
            {account.id}
          </code>
        </div>
        <p class="text-2xl font-semibold tracking-tight text-base-content tabular-nums">
          {formatMoney(account.balance, account.currency)}
        </p>
      </div>
    </div>
  {/if}

  {#if error && account}
    <div class="mt-6">
      <Alert variant="error">
        {error}
        <button type="button" class="btn btn-link ml-2 min-h-11" onclick={retry}>Retry</button>
      </Alert>
    </div>
  {:else if error}
    <div class="mt-6">
      <Alert variant="error">
        {error}
        <button type="button" class="btn btn-link ml-2 min-h-11" onclick={retry}>Retry</button>
      </Alert>
    </div>
  {:else if loading && !account}
    <div class="mt-6 flex flex-col gap-3" aria-busy="true" aria-label="Loading activity">
      {#each [0, 1, 2, 3] as placeholder (placeholder)}
        <div class="skeleton h-20"></div>
      {/each}
    </div>
  {/if}

  {#if account && transfers.length === 0 && !loading}
    <div class="card mt-6 border border-dashed border-base-300 bg-base-100 text-center">
      <div class="card-body items-center px-6 py-12">
        <h2 class="card-title text-base">No activity yet</h2>
        <p class="max-w-sm text-sm text-base-content/60">
          Transfers to and from this account will appear here.
        </p>
        <div class="card-actions mt-2">
          <Link href="/transfer" class="btn btn-primary min-h-11">Send money</Link>
        </div>
      </div>
    </div>
  {:else if account && transfers.length > 0}
    <ul
      class="list mt-6 rounded-box border border-base-300 bg-base-100 shadow-sm"
      aria-busy={refreshing}
    >
      {#each transfers as transfer (transfer.id)}
        {@const r = row(transfer)}
        <li class="list-row items-center">
          {#if r.outgoing}
            <ArrowUpRight size={18} class="shrink-0 text-error" aria-hidden="true" />
          {:else}
            <ArrowDownLeft size={18} class="shrink-0 text-success" aria-hidden="true" />
          {/if}
          <div class="min-w-0">
            <p class="text-sm font-semibold text-base-content">
              {r.outgoing ? "Sent" : "Received"}
            </p>
            <p class="mt-0.5 text-xs text-base-content/60">
              {r.outgoing ? "To" : "From"}
              <code class="font-mono break-words">{r.counterparty}</code>
            </p>
            <p class="mt-0.5 text-xs text-base-content/60">{r.when}</p>
          </div>
          <p
            class="shrink-0 text-sm font-semibold tabular-nums"
            class:text-error={r.outgoing}
            class:text-success={!r.outgoing}
          >
            {formatSignedMoney(r.signed, account.currency)}
          </p>
        </li>
      {/each}
    </ul>
  {/if}
</div>
