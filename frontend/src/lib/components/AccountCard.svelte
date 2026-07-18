<script lang="ts">
  import { formatMoney } from "../money";
  import type { Account } from "../types";

  interface Props {
    account: Account;
  }

  let { account }: Props = $props();
</script>

<article
  class="flex flex-col gap-4 rounded-card border border-border bg-surface p-5 transition-shadow hover:shadow-sm"
>
  <div class="flex items-center justify-between">
    <div>
      <h3 class="text-sm font-medium text-muted">{account.owner}</h3>
      <p class="font-mono text-xs text-muted">•••• {account.id.slice(-4)}</p>
    </div>
    <span
      class="rounded-full bg-surface-muted px-2.5 py-1 text-xs font-semibold tracking-wide text-ink"
    >
      {account.currency}
    </span>
  </div>

  <p class="text-2xl font-semibold tracking-tight text-ink tabular-nums">
    {formatMoney(account.balance, account.currency)}
  </p>

  <div class="flex gap-2">
    <a
      href="#transfer"
      class="flex-1 rounded-md bg-brand px-3 py-2 text-center text-sm font-medium text-surface transition-colors hover:bg-brand-strong"
    >
      Transfer
    </a>
    <a
      href="#account-{account.id}"
      class="flex-1 rounded-md border border-border px-3 py-2 text-center text-sm font-medium text-ink transition-colors hover:bg-surface-muted"
    >
      Details
    </a>
  </div>
</article>
