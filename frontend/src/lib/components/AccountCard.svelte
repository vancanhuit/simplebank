<script lang="ts">
  import type { Account } from "../api/types";
  import { formatMoney } from "../money";
  import { accounts } from "../stores/accounts.svelte";
  import { navigate } from "../router.svelte";

  interface Props {
    account: Account;
  }

  let { account }: Props = $props();

  let copied = $state(false);
  let copyTimer: ReturnType<typeof setTimeout> | undefined;

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

<article class="flex flex-col gap-4 rounded-card border border-border bg-surface p-5">
  <div class="flex items-start justify-between">
    <span
      class="inline-flex items-center rounded-full bg-brand-soft px-2.5 py-0.5 text-xs font-semibold text-brand-strong"
    >
      {account.currency}
    </span>
    <span class="text-xs text-muted">Opened {created}</span>
  </div>

  <p class="text-2xl font-semibold tracking-tight text-ink tabular-nums">
    {formatMoney(account.balance, account.currency)}
  </p>

  <div>
    <p class="text-xs font-medium text-muted">Account number</p>
    <div class="mt-1 flex items-start gap-2">
      <code class="min-w-0 flex-1 font-mono text-xs break-all text-ink">{account.id}</code>
      <button
        type="button"
        onclick={copyId}
        class="shrink-0 rounded-md border border-border px-2 py-1 text-xs font-medium text-brand transition-colors hover:bg-brand-soft"
        aria-label={copied ? "Account number copied" : "Copy account number"}
      >
        {copied ? "Copied" : "Copy"}
      </button>
    </div>
  </div>

  <button
    type="button"
    class="self-start text-sm font-semibold text-brand transition-colors hover:text-brand-strong"
    onclick={sendFromHere}
  >
    Send money →
  </button>
</article>
