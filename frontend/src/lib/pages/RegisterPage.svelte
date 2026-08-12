<script lang="ts">
  import { toMessage } from "../api/client";
  import { auth } from "../stores/auth.svelte";
  import { navigate } from "../router.svelte";
  import AuthLayout from "./AuthLayout.svelte";
  import TextField from "../components/TextField.svelte";
  import Button from "../components/Button.svelte";
  import Alert from "../components/Alert.svelte";
  import Link from "../components/Link.svelte";

  let fullName = $state("");
  let username = $state("");
  let email = $state("");
  let password = $state("");
  let error = $state<string | null>(null);
  let submitting = $state(false);

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    error = null;
    submitting = true;
    try {
      await auth.register({
        full_name: fullName.trim(),
        username: username.trim(),
        email: email.trim(),
        password,
      });
      // Registration returns 202 Accepted; send the user to sign in with a
      // one-shot success notice.
      navigate("/login", { registered: true });
    } catch (err) {
      error = toMessage(err);
    } finally {
      submitting = false;
    }
  }
</script>

<AuthLayout title="Create your account" subtitle="Open a SimpleBank account in seconds.">
  <form class="flex flex-col gap-4" onsubmit={handleSubmit} novalidate>
    {#if error}
      <Alert variant="error">{error}</Alert>
    {/if}

    <TextField label="Full name" bind:value={fullName} autocomplete="name" required />
    <TextField
      label="Username"
      bind:value={username}
      autocomplete="username"
      hint="Letters and numbers only."
      required
    />
    <TextField label="Email" type="email" bind:value={email} autocomplete="email" required />
    <TextField
      label="Password"
      type="password"
      bind:value={password}
      autocomplete="new-password"
      hint="At least 6 characters."
      required
    />

    <Button type="submit" loading={submitting} class="mt-2">Create account</Button>
  </form>

  {#snippet footer()}
    Already have an account?
    <Link href="/login" class="font-semibold text-brand hover:text-brand-strong">Sign in</Link>
  {/snippet}
</AuthLayout>
