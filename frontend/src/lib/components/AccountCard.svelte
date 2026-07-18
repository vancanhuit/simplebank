<script lang="ts">
  import type { Account } from "../api/types";
  import { formatMoney } from "../money";
  import { accounts } from "../stores/accounts.svelte";
  import { navigate } from "../router.svelte";

  interface Props {
    account: Account;
  }

  let { account }: Props = $props();

  // Show only the last segment of the UUID so the card is scannable without
  // spreading the full identifier across the screen.
  const shortId = $derived(account.id.split("-").at(-1) ?? account.id);

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
</script>

<article class="flex flex-col gap-4 rounded-card border border-border bg-surface p-5">
  <div class="flex items-start justify-between">
    <div>
      <span
        class="inline-flex items-center rounded-full bg-brand-soft px-2.5 py-0.5 text-xs font-semibold text-brand-strong"
      >
        {account.currency}
      </span>
      <p class="mt-2 font-mono text-xs text-muted">•••• {shortId}</p>
    </div>
    <span class="text-xs text-muted">Opened {created}</span>
  </div>

  <p class="text-2xl font-semibold tracking-tight text-ink tabular-nums">
    {formatMoney(account.balance, account.currency)}
  </p>

  <button
    type="button"
    class="self-start text-sm font-semibold text-brand transition-colors hover:text-brand-strong"
    onclick={sendFromHere}
  >
    Send money →
  </button>
</article>
