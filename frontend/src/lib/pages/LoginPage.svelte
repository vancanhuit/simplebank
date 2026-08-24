<script lang="ts">
  import { tick } from "svelte";
  import { toMessage } from "../api/client";
  import { auth } from "../stores/auth.svelte";
  import { navigate, replaceNavigationState, router, safeReturnPath } from "../router.svelte";
  import AuthLayout from "./AuthLayout.svelte";
  import TextField from "../components/TextField.svelte";
  import Button from "../components/Button.svelte";
  import Alert from "../components/Alert.svelte";
  import Link from "../components/Link.svelte";
  import { validateLogin } from "../auth-validation";

  let username = $state("");
  let password = $state("");
  let usernameError = $state<string | undefined>();
  let passwordError = $state<string | undefined>();
  let error = $state<string | null>(null);
  let submitting = $state(false);
  let logoutFailed = $state(false);
  let sessionExpired = $state(false);
  let returnTo: string | null = null;

  const fieldIds = {
    username: "login-username",
    password: "login-password",
  } as const;

  // One-shot notice set by the register flow via history state.
  const historyState: unknown = window.history.state;
  const registered =
    typeof historyState === "object" &&
    historyState !== null &&
    "registered" in historyState &&
    historyState.registered === true;

  $effect(() => {
    const state = router.state;
    const hasConsumedState =
      "logoutFailed" in state || "returnTo" in state || "sessionExpired" in state;
    if (!hasConsumedState) return;

    if (state.logoutFailed === true) {
      logoutFailed = true;
    }
    if (state.sessionExpired === true) {
      sessionExpired = true;
    }
    const validatedReturnTo = safeReturnPath(state.returnTo);
    if (validatedReturnTo !== null) {
      returnTo = validatedReturnTo;
    }

    const remainingState = { ...state };
    delete remainingState.logoutFailed;
    delete remainingState.returnTo;
    delete remainingState.sessionExpired;
    replaceNavigationState(remainingState);
  });

  async function focusFirstInvalid(errors: { username?: string; password?: string }) {
    const firstInvalidId = errors.username
      ? fieldIds.username
      : errors.password
        ? fieldIds.password
        : undefined;
    if (!firstInvalidId) return;

    await tick();
    document.getElementById(firstInvalidId)?.focus();
  }

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    error = null;
    const validation = validateLogin({ username, password });
    usernameError = validation.errors.username;
    passwordError = validation.errors.password;
    if (Object.keys(validation.errors).length > 0) {
      await focusFirstInvalid(validation.errors);
      return;
    }

    submitting = true;
    try {
      await auth.login(validation.values.username, validation.values.password);
      navigate(returnTo ?? "/");
    } catch (err) {
      error = toMessage(err);
    } finally {
      submitting = false;
    }
  }
</script>

<AuthLayout title="Welcome back" subtitle="Sign in to your SimpleBank account.">
  <form class="flex flex-col gap-5" onsubmit={handleSubmit} novalidate>
    {#if logoutFailed}
      <Alert variant="error">
        You were signed out locally, but SimpleBank couldn't complete the server sign-out request.
      </Alert>
    {/if}
    {#if sessionExpired}
      <Alert>Your session expired. Sign in again to continue.</Alert>
    {/if}
    {#if registered}
      <Alert variant="success"
        >Account request accepted. Check your email to verify it, then sign in.</Alert
      >
    {/if}
    {#if error}
      <Alert variant="error">{error}</Alert>
    {/if}

    <TextField
      id={fieldIds.username}
      label="Username"
      bind:value={username}
      autocomplete="username"
      error={usernameError}
      oninput={() => (usernameError = undefined)}
      disabled={auth.loggingOut}
      required
    />
    <TextField
      id={fieldIds.password}
      label="Password"
      type="password"
      bind:value={password}
      autocomplete="current-password"
      error={passwordError}
      oninput={() => (passwordError = undefined)}
      disabled={auth.loggingOut}
      required
    />

    <Button type="submit" loading={submitting} disabled={auth.loggingOut} class="mt-2 w-full">
      Sign in
    </Button>
  </form>

  {#snippet footer()}
    Don't have an account?
    <Link href="/register" class="link link-primary font-semibold">Create one</Link>
  {/snippet}
</AuthLayout>
