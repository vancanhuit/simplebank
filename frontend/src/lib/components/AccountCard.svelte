<script lang="ts">
  import { onDestroy } from "svelte";
  import type { Account } from "../api/types";
  import { formatMoney } from "../money";
  import { accounts } from "../stores/accounts.svelte";
  import { navigate } from "../router.svelte";
  import Link from "./Link.svelte";
  import Copy from "@lucide/svelte/icons/copy";
  import Check from "@lucide/svelte/icons/check";
  import Send from "@lucide/svelte/icons/send";
  import History from "@lucide/svelte/icons/history";
  interface Props {
    account: Account;
  }

  let { account }: Props = $props();

  let copied = $state(false);
  let copyTimer: ReturnType<typeof setTimeout> | undefined;

  // Clear a pending reset if the card unmounts, so the timer never writes to a
  // destroyed instance.
  onDestroy(() => clearTimeout(copyTimer));

  const created = $derived(
    new Date(account.created_at).toLocaleDateString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
    }),
  );

  function sendFromHere() {
    accounts.transferFromId = account.id;
    navigate("/transfer");
  }

  async function copyId() {
    try {
      await navigator.clipboard.writeText(account.id);
      copied = true;
      clearTimeout(copyTimer);
      copyTimer = setTimeout(() => (copied = false), 2000);
    } catch {
      // Clipboard may be unavailable (e.g. insecure context); the account number
      // is still selectable, so no user-facing error is needed.
    }
  }
</script>

<article
  class="card card-border bg-base-100 shadow-sm transition-transform hover:-translate-y-0.5 motion-reduce:transform-none"
>
  <div class="card-body gap-5 p-5 sm:p-6">
    <div class="flex items-center justify-between gap-3">
      <span class="badge badge-outline font-semibold">{account.currency}</span>
      <span class="text-xs text-base-content/70">Opened {created}</span>
    </div>

    <div>
      <p class="text-xs font-medium tracking-wide text-base-content/70 uppercase">
        Available balance
      </p>
      <p class="mt-1 text-3xl font-semibold tracking-tight tabular-nums">
        {formatMoney(account.balance, account.currency)}
      </p>
    </div>

    <div class="rounded-box bg-base-200 p-3">
      <p class="text-xs font-medium text-base-content/70">Account number</p>
      <code class="mt-1 block font-mono text-xs break-all">{account.id}</code>
      <button
        type="button"
        onclick={copyId}
        class="btn btn-ghost btn-sm mt-2 min-h-11"
        aria-label={copied ? "Account number copied" : "Copy account number"}
      >
        {#if copied}
          <Check size={14} aria-hidden="true" />
        {:else}
          <Copy size={14} aria-hidden="true" />
        {/if}
        {copied ? "Copied" : "Copy"}
      </button>
    </div>

    <div class="card-actions grid grid-cols-2">
      <button type="button" class="btn min-h-11" onclick={sendFromHere}>
        <Send size={16} aria-hidden="true" />
        Send money
      </button>
      <Link href={`/accounts/${account.id}`} class="btn btn-ghost min-h-11">
        <History size={16} aria-hidden="true" />
        Activity
      </Link>
    </div>
  </div>
</article>
