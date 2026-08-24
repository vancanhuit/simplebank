<script lang="ts">
  import { onMount } from "svelte";
  import ArrowLeft from "@lucide/svelte/icons/arrow-left";
  import { request, toMessage } from "../api/client";
  import type { AccountOpeningLimits } from "../api/types";
  import { accounts } from "../stores/accounts.svelte";
  import {
    CURRENCIES,
    formatMoney,
    fractionDigits,
    parseAmountToMinor,
    type Currency,
  } from "../money";
  import { openingLimitFor, openingLimitInputMax, validateOpeningBalance } from "../opening-limits";
  import { navigate } from "../router.svelte";
  import Button from "../components/Button.svelte";
  import Alert from "../components/Alert.svelte";
  import TextField from "../components/TextField.svelte";
  import Link from "../components/Link.svelte";

  let currency = $state<Currency>("USD");
  let deposit = $state("");
  let error = $state<string | null>(null);
  let depositError = $state<string | null>(null);
  let submitting = $state(false);
  let openingLimits = $state<AccountOpeningLimits>({});
  let policyLoading = $state(true);
  let policyError = $state<string | null>(null);

  onMount(() => {
    void loadOpeningLimits();
    void loadAccounts();
  });

  async function loadAccounts(): Promise<void> {
    if (!accounts.loaded || accounts.error !== null) {
      await accounts.load();
    }

    if (accounts.loaded && !accounts.loading && accounts.error === null) {
      const firstAvailable = available[0];
      if (firstAvailable && !available.includes(currency)) {
        currency = firstAvailable;
      }
    }
  }

  // The API rejects a second account in a currency the user already holds, so
  // hide currencies already taken to prevent a guaranteed error.
  const available = $derived(
    CURRENCIES.filter((code) => !accounts.items.some((account) => account.currency === code)),
  );

  const depositStep = $derived(fractionDigits(currency) === 0 ? "1" : "0.01");
  const openingLimit = $derived(openingLimitFor(openingLimits, currency));
  const depositMax = $derived(openingLimitInputMax(openingLimit, currency));
  const policyReady = $derived(!policyLoading && policyError === null);
  const accountsReady = $derived(accounts.loaded && !accounts.loading && accounts.error === null);
  const formDisabled = $derived(!policyReady || !accountsReady || submitting);
  const depositHint = $derived(
    `Optional. Maximum ${formatMoney(openingLimit, currency)}. Leave blank to open at zero.`,
  );

  async function loadOpeningLimits(): Promise<void> {
    policyLoading = true;
    policyError = null;

    try {
      openingLimits = await request<AccountOpeningLimits>("/account-opening-limits");
    } catch (err) {
      openingLimits = {};
      policyError = `We couldn't load the account opening policy. ${toMessage(err)}`;
    } finally {
      policyLoading = false;
    }
  }

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    error = null;
    depositError = null;

    if (!policyReady) {
      error = "Account opening policy is unavailable. Retry once it finishes loading.";
      return;
    }

    // The opening deposit is optional; an empty field opens the account at zero.
    // A number-type input binds as a number at runtime, so coerce before testing.
    let balance = 0;
    if (String(deposit).trim() !== "") {
      const minor = parseAmountToMinor(deposit, currency);
      if (minor === null) {
        depositError = "Enter an opening deposit greater than zero, or leave it blank.";
        return;
      }
      balance = minor;
      const limitMessage = validateOpeningBalance(balance, currency, openingLimits);
      if (limitMessage) {
        depositError = limitMessage;
        return;
      }
    }

    submitting = true;
    try {
      await accounts.create(currency, balance);
      navigate("/");
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
  <h1 class="mt-6 text-3xl font-bold tracking-tight text-base-content sm:text-4xl">
    Open a new account
  </h1>
  <p class="mt-2 text-base text-base-content/65">Choose a currency for your new account.</p>

  <div class="card card-border mt-8 bg-base-100 shadow-sm">
    <form class="card-body gap-5 p-6 sm:p-8" onsubmit={handleSubmit}>
      {#if error}
        <Alert variant="error">{error}</Alert>
      {/if}

      {#if policyError}
        <Alert variant="error">
          {policyError}
          <button type="button" class="btn btn-ghost ml-2 min-h-11" onclick={loadOpeningLimits}
            >Retry</button
          >
        </Alert>
      {:else if policyLoading}
        <Alert variant="info">Loading the account opening policy…</Alert>
      {/if}

      {#if !accountsReady}
        <div aria-busy={accounts.loading}>
          {#if accounts.error}
            <Alert variant="error">
              We couldn't load your accounts. {accounts.error}
              <button type="button" class="btn btn-ghost min-h-11" onclick={loadAccounts}
                >Retry</button
              >
            </Alert>
          {:else}
            <Alert variant="info">
              <span class="loading loading-spinner loading-sm" aria-hidden="true"></span>
              Loading your accounts…
            </Alert>
          {/if}
        </div>
      {:else if available.length === 0}
        <Alert variant="info">You already hold an account in every supported currency.</Alert>
      {:else}
        <fieldset class="fieldset gap-3" aria-busy={policyLoading}>
          <legend class="fieldset-legend">Currency</legend>
          <div class="grid gap-3 sm:grid-cols-3">
            {#each available as code (code)}
              <label
                class="label min-h-20 cursor-pointer rounded-box border border-base-300 bg-base-100 p-4 has-[:checked]:border-primary has-[:checked]:bg-primary/10"
              >
                <span>
                  <span class="block font-semibold">{code}</span>
                  <span class="text-xs text-base-content/60">
                    Starts at {formatMoney(0, code)}
                  </span>
                </span>
                <input
                  type="radio"
                  name="currency"
                  value={code}
                  bind:group={currency}
                  disabled={formDisabled}
                  class="radio radio-primary"
                />
              </label>
            {/each}
          </div>
        </fieldset>

        <TextField
          label={`Opening deposit (${currency})`}
          type="number"
          inputmode="decimal"
          step={depositStep}
          min="0"
          max={depositMax}
          bind:value={deposit}
          placeholder="0.00"
          hint={depositHint}
          error={depositError ?? undefined}
          disabled={formDisabled}
        />

        <Button
          type="submit"
          loading={submitting}
          disabled={formDisabled || available.length === 0}
          class="mt-2 w-full sm:w-auto"
        >
          Create account
        </Button>
      {/if}
    </form>
  </div>
</div>
