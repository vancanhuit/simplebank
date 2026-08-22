<script lang="ts">
  import { tick } from "svelte";
  import { toMessage } from "../api/client";
  import { auth } from "../stores/auth.svelte";
  import { navigate } from "../router.svelte";
  import AuthLayout from "./AuthLayout.svelte";
  import TextField from "../components/TextField.svelte";
  import Button from "../components/Button.svelte";
  import Alert from "../components/Alert.svelte";
  import Link from "../components/Link.svelte";
  import { validateRegistration } from "../auth-validation";

  let fullName = $state("");
  let username = $state("");
  let email = $state("");
  let password = $state("");
  let fullNameError = $state<string | undefined>();
  let usernameError = $state<string | undefined>();
  let emailError = $state<string | undefined>();
  let passwordError = $state<string | undefined>();
  let error = $state<string | null>(null);
  let submitting = $state(false);

  const fieldIds = {
    fullName: "register-full-name",
    username: "register-username",
    email: "register-email",
    password: "register-password",
  } as const;

  async function focusFirstInvalid(errors: {
    fullName?: string;
    username?: string;
    email?: string;
    password?: string;
  }) {
    const firstInvalidId = errors.fullName
      ? fieldIds.fullName
      : errors.username
        ? fieldIds.username
        : errors.email
          ? fieldIds.email
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
    const validation = validateRegistration({ fullName, username, email, password });
    fullNameError = validation.errors.fullName;
    usernameError = validation.errors.username;
    emailError = validation.errors.email;
    passwordError = validation.errors.password;
    if (Object.keys(validation.errors).length > 0) {
      await focusFirstInvalid(validation.errors);
      return;
    }

    submitting = true;
    try {
      await auth.register({
        full_name: validation.values.fullName,
        username: validation.values.username,
        email: validation.values.email,
        password: validation.values.password,
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
  <form class="flex flex-col gap-5" onsubmit={handleSubmit} novalidate>
    {#if error}
      <Alert variant="error">{error}</Alert>
    {/if}

    <TextField
      id={fieldIds.fullName}
      label="Full name"
      bind:value={fullName}
      autocomplete="name"
      error={fullNameError}
      oninput={() => (fullNameError = undefined)}
      required
    />
    <TextField
      id={fieldIds.username}
      label="Username"
      bind:value={username}
      autocomplete="username"
      hint="Letters and numbers only."
      error={usernameError}
      oninput={() => (usernameError = undefined)}
      required
    />
    <TextField
      id={fieldIds.email}
      label="Email"
      type="email"
      bind:value={email}
      autocomplete="email"
      error={emailError}
      oninput={() => (emailError = undefined)}
      required
    />
    <TextField
      id={fieldIds.password}
      label="Password"
      type="password"
      bind:value={password}
      autocomplete="new-password"
      hint="At least 15 characters."
      error={passwordError}
      oninput={() => (passwordError = undefined)}
      required
    />

    <Button type="submit" loading={submitting} class="mt-2 w-full">Create account</Button>
  </form>

  {#snippet footer()}
    Already have an account?
    <Link href="/login" class="link link-primary font-semibold">Sign in</Link>
  {/snippet}
</AuthLayout>
