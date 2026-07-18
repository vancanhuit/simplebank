<script lang="ts">
  import { onMount } from "svelte";
  import { toMessage } from "../api/client";
  import { accounts } from "../stores/accounts.svelte";
  import { CURRENCIES, formatMoney, type Currency } from "../money";
  import { navigate } from "../router.svelte";
  import Button from "../components/Button.svelte";
  import Alert from "../components/Alert.svelte";
  import Link from "../components/Link.svelte";

  let currency = $state<Currency>("USD");
  let error = $state<string | null>(null);
  let submitting = $state(false);

  onMount(() => {
    if (!accounts.loaded) {
      void accounts.load();
    }
  });

  // The API rejects a second account in a currency the user already holds, so
  // hide currencies already taken to prevent a guaranteed error.
  const available = $derived(
    CURRENCIES.filter((code) => !accounts.items.some((account) => account.currency === code)),
  );

  $effect(() => {
    if (available.length > 0 && !available.includes(currency)) {
      currency = available[0];
    }
  });

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    error = null;
    submitting = true;
    try {
      await accounts.create(currency);
      navigate("/");
    } catch (err) {
      error = toMessage(err);
    } finally {
      submitting = false;
    }
  }
</script>

<div class="mx-auto max-w-lg">
  <Link href="/" class="text-sm font-medium text-brand hover:text-brand-strong">← Back</Link>
  <h1 class="mt-4 text-2xl font-semibold text-ink">Open a new account</h1>
  <p class="mt-1 text-sm text-muted">Choose a currency for your new account.</p>

  <form class="mt-6 flex flex-col gap-5" onsubmit={handleSubmit}>
    {#if error}
      <Alert variant="error">{error}</Alert>
    {/if}

    {#if available.length === 0}
      <Alert variant="info">You already hold an account in every supported currency.</Alert>
    {:else}
      <fieldset class="flex flex-col gap-3">
        <legend class="text-sm font-medium text-ink">Currency</legend>
        {#each available as code (code)}
          <label
            class="flex cursor-pointer items-center gap-3 rounded-md border border-border bg-surface px-4 py-3 text-sm has-[:checked]:border-brand has-[:checked]:bg-brand-soft/40"
          >
            <input
              type="radio"
              name="currency"
              value={code}
              bind:group={currency}
              class="accent-brand"
            />
            <span class="font-semibold text-ink">{code}</span>
            <span class="ml-auto text-muted">Starts at {formatMoney(0, code)}</span>
          </label>
        {/each}
      </fieldset>

      <Button type="submit" loading={submitting}>Create account</Button>
    {/if}
  </form>
</div>
