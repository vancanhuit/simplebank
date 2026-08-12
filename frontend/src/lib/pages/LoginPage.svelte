<script lang="ts">
  import { toMessage } from "../api/client";
  import { auth } from "../stores/auth.svelte";
  import { navigate } from "../router.svelte";
  import AuthLayout from "./AuthLayout.svelte";
  import TextField from "../components/TextField.svelte";
  import Button from "../components/Button.svelte";
  import Alert from "../components/Alert.svelte";
  import Link from "../components/Link.svelte";

  let username = $state("");
  let password = $state("");
  let error = $state<string | null>(null);
  let submitting = $state(false);

  // One-shot notice set by the register flow via history state.
  const registered =
    typeof history.state === "object" &&
    history.state !== null &&
    history.state.registered === true;

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    error = null;
    submitting = true;
    try {
      await auth.login(username.trim(), password);
      navigate("/");
    } catch (err) {
      error = toMessage(err);
    } finally {
      submitting = false;
    }
  }
</script>

<AuthLayout title="Welcome back" subtitle="Sign in to your SimpleBank account.">
  <form class="flex flex-col gap-4" onsubmit={handleSubmit} novalidate>
    {#if registered}
      <Alert variant="success"
        >Account request accepted. Check your email to verify it, then sign in.</Alert
      >
    {/if}
    {#if error}
      <Alert variant="error">{error}</Alert>
    {/if}

    <TextField label="Username" bind:value={username} autocomplete="username" required />
    <TextField
      label="Password"
      type="password"
      bind:value={password}
      autocomplete="current-password"
      required
    />

    <Button type="submit" loading={submitting} class="mt-2">Sign in</Button>
  </form>

  {#snippet footer()}
    Don't have an account?
    <Link href="/register" class="font-semibold text-brand hover:text-brand-strong">
      Create one
    </Link>
  {/snippet}
</AuthLayout>
