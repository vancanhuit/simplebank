<script lang="ts">
  import { onMount } from "svelte";
  import { request, toMessage } from "../api/client";
  import type { TransferLimits, TransferResult } from "../api/types";
  import { accounts } from "../stores/accounts.svelte";
  import { formatMoney, fractionDigits, parseAmountToMinor } from "../money";
  import Button from "../components/Button.svelte";
  import Alert from "../components/Alert.svelte";
  import TextField from "../components/TextField.svelte";
  import Link from "../components/Link.svelte";

  let fromAccountId = $state("");
  let toAccountId = $state("");
  let amount = $state("");
  let error = $state<string | null>(null);
  let toError = $state<string | null>(null);
  let amountError = $state<string | null>(null);
  let submitting = $state(false);
  let receipt = $state<TransferResult | null>(null);
  let limits = $state<TransferLimits>({});

  onMount(async () => {
    if (!accounts.loaded) {
      await accounts.load();
    }
    // Preselect the account chosen from a card, then the first account.
    fromAccountId = accounts.transferFromId ?? accounts.items[0]?.id ?? "";
    accounts.transferFromId = null;
    // Load the per-currency limits so we can flag an over-limit amount before
    // hitting the API. A failure here is non-fatal: the server still enforces.
    try {
      limits = await request<TransferLimits>("/transfer-limits");
    } catch {
      limits = {};
    }
  });

  const fromAccount = $derived(accounts.get(fromAccountId));
  const amountStep = $derived(
    fromAccount ? (fractionDigits(fromAccount.currency) === 0 ? "1" : "0.01") : "0.01",
  );

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    error = null;
    toError = null;
    amountError = null;
    receipt = null;

    if (!fromAccount) {
      error = "Choose an account to send from.";
      return;
    }
    const recipient = toAccountId.trim();
    if (!recipient) {
      toError = "Enter the recipient account id.";
      return;
    }
    if (recipient === fromAccount.id) {
      toError = "Choose a different recipient account.";
      return;
    }
    const minor = parseAmountToMinor(amount, fromAccount.currency);
    if (minor === null) {
      amountError = "Enter an amount greater than zero.";
      return;
    }
    // Mirror the server's per-currency ceiling so the user gets immediate
    // feedback instead of a round-trip rejection.
    const limit = limits[fromAccount.currency];
    if (limit && limit.max_per_transfer > 0 && minor > limit.max_per_transfer) {
      amountError = `Amount exceeds the ${formatMoney(limit.max_per_transfer, fromAccount.currency)} per-transfer limit.`;
      return;
    }

    submitting = true;
    try {
      const result = await request<TransferResult>("/transfers", {
        method: "POST",
        authenticated: true,
        body: {
          from_account_id: fromAccount.id,
          to_account_id: recipient,
          amount: minor,
          currency: fromAccount.currency,
          // A fresh key per submit makes the request safe to retry at the
          // network layer without moving money twice. The `submitting` guard
          // already blocks a second concurrent submit of the same form.
          idempotency_key: crypto.randomUUID(),
        },
      });
      receipt = result;
      // Reload balances so the dashboard and the from-account reflect the debit.
      await accounts.load();
      amount = "";
      toAccountId = "";
    } catch (err) {
      error = toMessage(err);
    } finally {
      submitting = false;
    }
  }
</script>

<div class="mx-auto max-w-lg">
  <Link href="/" class="text-sm font-medium text-brand hover:text-brand-strong">← Back</Link>
  <h1 class="mt-4 text-2xl font-semibold text-ink">Send money</h1>
  <p class="mt-1 text-sm text-muted">Transfer funds to another SimpleBank account.</p>

  {#if receipt}
    <div class="mt-6">
      <Alert variant="success">
        Sent {formatMoney(receipt.transfer.amount, receipt.from_account.currency)} successfully.
      </Alert>
    </div>
    <dl
      class="mt-4 grid grid-cols-2 gap-4 rounded-card border border-border bg-surface p-5 text-sm"
    >
      <div>
        <dt class="text-muted">From</dt>
        <dd class="mt-0.5 font-medium text-ink">
          {formatMoney(receipt.from_account.balance, receipt.from_account.currency)} left
        </dd>
      </div>
      <div>
        <dt class="text-muted">Reference</dt>
        <dd class="mt-0.5 font-mono text-xs break-all text-ink">{receipt.transfer.id}</dd>
      </div>
    </dl>
  {/if}

  {#if accounts.items.length === 0 && accounts.loaded}
    <div class="mt-6">
      <Alert variant="info">
        You need an account before you can send money.
        <Link href="/accounts/new" class="font-semibold underline">Open one</Link>.
      </Alert>
    </div>
  {:else}
    <form class="mt-6 flex flex-col gap-5" onsubmit={handleSubmit} novalidate>
      {#if error}
        <Alert variant="error">{error}</Alert>
      {/if}

      <div class="flex flex-col gap-1.5">
        <label for="from" class="text-sm font-medium text-ink">From account</label>
        <select
          id="from"
          bind:value={fromAccountId}
          class="rounded-md border border-border bg-surface px-3 py-2.5 text-sm text-ink focus-visible:border-brand"
        >
          {#each accounts.items as account (account.id)}
            <option value={account.id}>
              {account.currency} · {formatMoney(account.balance, account.currency)}
            </option>
          {/each}
        </select>
      </div>

      <TextField
        label="Recipient account id"
        bind:value={toAccountId}
        placeholder="00000000-0000-0000-0000-000000000000"
        hint="The recipient's account must use the same currency."
        error={toError ?? undefined}
        required
      />

      <TextField
        label={`Amount${fromAccount ? ` (${fromAccount.currency})` : ""}`}
        type="number"
        inputmode="decimal"
        step={amountStep}
        min="0"
        bind:value={amount}
        placeholder="0.00"
        error={amountError ?? undefined}
        required
      />

      <Button type="submit" loading={submitting}>Send transfer</Button>
    </form>
  {/if}
</div>
