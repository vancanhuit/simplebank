<script lang="ts">
  import { onMount } from "svelte";
  import CircleCheck from "@lucide/svelte/icons/circle-check";
  import CircleAlert from "@lucide/svelte/icons/circle-alert";
  import { isRetryable, request, toMessage } from "../api/client";
  import { verificationResponse } from "../api/validation";
  import { auth } from "../stores/auth.svelte";
  import AuthLayout from "./AuthLayout.svelte";
  import Link from "../components/Link.svelte";
  import Alert from "../components/Alert.svelte";

  type Status = "pending" | "success" | "error";

  let status = $state<Status>("pending");
  let errorMessage = $state("");
  let retryable = $state(false);
  let verification = $state<{ id: string; code: string } | null>(null);

  async function verify(id: string, code: string): Promise<void> {
    status = "pending";
    errorMessage = "";
    retryable = false;
    try {
      const query = `?id=${encodeURIComponent(id)}&code=${encodeURIComponent(code)}`;
      verificationResponse(await request<unknown>(`/users/verify_email${query}`));
      status = "success";
    } catch (err) {
      status = "error";
      errorMessage = toMessage(err);
      retryable = isRetryable(err);
    }
  }

  function retryVerification(): void {
    if (verification !== null) {
      void verify(verification.id, verification.code);
    }
  }

  // The verification link carries the record id and secret code as query
  // params. The router tracks only the pathname, so read them from the URL.
  onMount(async () => {
    const params = new URLSearchParams(window.location.search);
    const id = params.get("id") ?? "";
    const code = params.get("code") ?? "";

    // Keep the one-time credential out of browser history after capturing it.
    window.history.replaceState(null, "", window.location.pathname);

    if (!id || !code) {
      status = "error";
      errorMessage = "This verification link is incomplete. Please use the link from your email.";
      return;
    }
    verification = { id, code };
    await verify(id, code);
  });

  // Signed-in visitors go back to the dashboard; everyone else signs in.
  const destination = $derived(auth.isAuthenticated ? "/" : "/login");
  const destinationLabel = $derived(
    auth.isAuthenticated ? "Go to dashboard" : "Continue to sign in",
  );
</script>

<AuthLayout title="Email verification" subtitle="Confirming your SimpleBank email address.">
  {#if status === "pending"}
    <div class="flex flex-col items-center gap-4 py-4 text-center" role="status" aria-busy="true">
      <span class="loading loading-ring loading-lg text-primary" aria-hidden="true"></span>
      <p class="text-sm text-base-content/65">Verifying your email…</p>
    </div>
  {:else if status === "success"}
    <div class="flex flex-col items-center gap-4 py-4 text-center" role="status">
      <span
        class="grid h-12 w-12 place-items-center rounded-full bg-success/15 text-success"
        aria-hidden="true"
      >
        <CircleCheck class="h-6 w-6" aria-hidden="true" />
      </span>
      <div>
        <h2 class="text-base font-semibold">Email verified</h2>
        <p class="mt-1 text-sm text-base-content/65">
          Thanks for confirming your address. Your account is ready to use.
        </p>
      </div>
      <Link href={destination} class="btn btn-primary mt-2">
        {destinationLabel}
      </Link>
    </div>
  {:else}
    <div class="flex flex-col items-center gap-4 py-4 text-center">
      <span
        class="grid h-12 w-12 place-items-center rounded-full bg-error/15 text-error"
        aria-hidden="true"
      >
        <CircleAlert class="h-6 w-6" aria-hidden="true" />
      </span>
      <h2 class="text-base font-semibold">Verification failed</h2>
      <Alert variant="error">{errorMessage}</Alert>
      {#if retryable && verification}
        <button type="button" class="btn mt-2" onclick={retryVerification}>Retry</button>
      {:else}
        <Link href="/login" class="btn btn-outline mt-2">Continue to sign in</Link>
      {/if}
    </div>
  {/if}

  {#snippet footer()}
    Need help? Sign in and request a new verification email.
  {/snippet}
</AuthLayout>
