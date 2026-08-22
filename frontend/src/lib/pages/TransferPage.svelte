<script lang="ts">
  import { onMount } from "svelte";
  import ArrowLeft from "@lucide/svelte/icons/arrow-left";
  import { request, toMessage } from "../api/client";
  import type { TransferLimits, TransferResult } from "../api/types";
  import { accounts } from "../stores/accounts.svelte";
  import { formatMoney, fractionDigits, parseAmountToMinor, type Currency } from "../money";
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
  interface TransferIntent {
    from_account_id: string;
    to_account_id: string;
    amount: number;
    currency: Currency;
  }

  // One key per validated transfer intent. Failed unchanged retries retain the
  // binding; a changed validated intent rotates before it reaches the API.
  let idempotencyKey = crypto.randomUUID();
  let keyedIntent: TransferIntent | null = null;

  function sameIntent(left: TransferIntent, right: TransferIntent): boolean {
    return (
      left.from_account_id === right.from_account_id &&
      left.to_account_id === right.to_account_id &&
      left.amount === right.amount &&
      left.currency === right.currency
    );
  }

  onMount(() => {
    void loadAccounts();
    void loadTransferLimits();
  });

  async function loadAccounts(): Promise<void> {
    if (!accounts.loaded || accounts.error !== null) {
      await accounts.load();
    }

    if (accounts.loaded && !accounts.loading && accounts.error === null) {
      // Preselect the account chosen from a card, then the first account.
      fromAccountId = accounts.transferFromId ?? accounts.items[0]?.id ?? "";
      accounts.transferFromId = null;
    }
  }

  async function loadTransferLimits(): Promise<void> {
    // Load the per-currency limits so we can flag an over-limit amount before
    // hitting the API. A failure here is non-fatal: the server still enforces.
    try {
      limits = await request<TransferLimits>("/transfer-limits");
    } catch {
      limits = {};
    }
  }

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

    const intent: TransferIntent = {
      from_account_id: fromAccount.id,
      to_account_id: recipient,
      amount: minor,
      currency: fromAccount.currency,
    };
    if (keyedIntent && !sameIntent(keyedIntent, intent)) {
      idempotencyKey = crypto.randomUUID();
    }
    keyedIntent = intent;

    submitting = true;
    try {
      const result = await request<TransferResult>("/transfers", {
        method: "POST",
        authenticated: true,
        body: {
          ...intent,
          // Stable across retries of this same transfer: if the response is lost
          // but the server committed, resubmitting replays it rather than
          // debiting twice.
          idempotency_key: idempotencyKey,
        },
      });
      receipt = result;
      // The transfer is confirmed, so the next one is a new intent: rotate the
      // key and clear the form.
      idempotencyKey = crypto.randomUUID();
      keyedIntent = null;
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

<div class="mx-auto max-w-2xl">
  <Link href="/" class="btn btn-ghost -ml-4 min-h-11 gap-2">
    <ArrowLeft class="h-4 w-4" aria-hidden="true" />
    Back
  </Link>
  <h1 class="mt-6 text-3xl font-bold tracking-tight text-base-content sm:text-4xl">Send money</h1>
  <p class="mt-2 text-base text-base-content/65">Transfer funds to another SimpleBank account.</p>

  {#if receipt}
    <div role="status" class="card card-border mt-8 border-success/30 bg-success/10">
      <div class="card-body gap-5 p-6 sm:p-8">
        <p class="font-semibold text-success">
          Sent {formatMoney(receipt.transfer.amount, receipt.from_account.currency)} successfully.
        </p>
        <dl class="grid gap-4 text-sm sm:grid-cols-3">
          <div>
            <dt class="text-base-content/60">Amount</dt>
            <dd class="mt-1 font-medium text-base-content">
              {formatMoney(receipt.transfer.amount, receipt.from_account.currency)}
            </dd>
          </div>
          <div>
            <dt class="text-base-content/60">From account</dt>
            <dd class="mt-1 font-medium text-base-content">
              Remaining balance: {formatMoney(
                receipt.from_account.balance,
                receipt.from_account.currency,
              )} left
            </dd>
          </div>
          <div>
            <dt class="text-base-content/60">Reference</dt>
            <dd class="mt-1 font-mono text-xs break-all text-base-content">
              {receipt.transfer.id}
            </dd>
          </div>
        </dl>
      </div>
    </div>
  {/if}

  {#if !accounts.loaded || accounts.loading || accounts.error}
    <div class="mt-8" aria-busy={accounts.loading}>
      {#if accounts.error}
        <Alert variant="error">
          Couldn't load your accounts. {accounts.error}
          <button type="button" class="btn btn-ghost min-h-11" onclick={loadAccounts}>Retry</button>
        </Alert>
      {:else}
        <Alert variant="info">
          <span class="loading loading-spinner loading-sm" aria-hidden="true"></span>
          Loading your accounts…
        </Alert>
      {/if}
    </div>
  {:else if accounts.items.length === 0}
    <div class="mt-8">
      <Alert variant="info">
        You need an account before you can send money.
        <Link href="/accounts/new" class="font-semibold underline">Open one</Link>.
      </Alert>
    </div>
  {:else}
    <div class="card card-border mt-8 bg-base-100 shadow-sm">
      <form class="card-body gap-5 p-6 sm:p-8" onsubmit={handleSubmit} novalidate>
        {#if error}
          <Alert variant="error">{error}</Alert>
        {/if}

        <fieldset class="fieldset">
          <label for="from" class="fieldset-legend">From account</label>
          <select id="from" bind:value={fromAccountId} class="select w-full min-h-11">
            {#each accounts.items as account (account.id)}
              <option value={account.id}>
                {account.currency} · {formatMoney(account.balance, account.currency)}
              </option>
            {/each}
          </select>
          {#if fromAccount}
            <p class="label text-base-content/65">
              Available: {formatMoney(fromAccount.balance, fromAccount.currency)}
            </p>
          {/if}
        </fieldset>

        <TextField
          label="Recipient account id"
          bind:value={toAccountId}
          placeholder="00000000-0000-0000-0000-000000000000"
          hint="The recipient's account must use the same currency."
          error={toError ?? undefined}
          oninput={() => (toError = null)}
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
          oninput={() => (amountError = null)}
          required
        />

        <Button type="submit" loading={submitting} class="mt-2 w-full sm:w-auto">
          Send transfer
        </Button>
      </form>
    </div>
  {/if}
</div>
