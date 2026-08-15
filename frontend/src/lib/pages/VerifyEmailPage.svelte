<script lang="ts">
  import { onMount } from "svelte";
  import { request, toMessage } from "../api/client";
  import { auth } from "../stores/auth.svelte";
  import AuthLayout from "./AuthLayout.svelte";
  import Link from "../components/Link.svelte";

  type Status = "pending" | "success" | "error";

  let status = $state<Status>("pending");
  let errorMessage = $state("");

  // The verification link carries the record id and secret code as query
  // params. The router tracks only the pathname, so read them from the URL.
  onMount(async () => {
    const params = new URLSearchParams(window.location.search);
    const id = params.get("id") ?? "";
    const code = params.get("code") ?? "";

    if (!id || !code) {
      status = "error";
      errorMessage = "This verification link is incomplete. Please use the link from your email.";
      return;
    }

    try {
      const query = `?id=${encodeURIComponent(id)}&code=${encodeURIComponent(code)}`;
      await request<{ is_verified: boolean }>(`/users/verify_email${query}`);
      status = "success";
    } catch (err) {
      status = "error";
      errorMessage = toMessage(err);
    }
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
      <span
        class="h-8 w-8 animate-spin rounded-full border-2 border-brand border-t-transparent"
        aria-hidden="true"
      ></span>
      <p class="text-sm text-muted">Verifying your email…</p>
    </div>
  {:else if status === "success"}
    <div class="flex flex-col items-center gap-4 py-4 text-center" role="status">
      <span
        class="grid h-12 w-12 place-items-center rounded-full bg-positive-soft text-positive"
        aria-hidden="true"
      >
        <svg viewBox="0 0 24 24" class="h-6 w-6" fill="none" stroke="currentColor" stroke-width="2">
          <path d="m5 13 4 4L19 7" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
      </span>
      <div>
        <h2 class="text-base font-semibold text-ink">Email verified</h2>
        <p class="mt-1 text-sm text-muted">
          Thanks for confirming your address. Your account is ready to use.
        </p>
      </div>
      <Link
        href={destination}
        class="mt-2 inline-flex min-h-11 items-center justify-center rounded-md bg-brand px-4 py-2 text-sm font-semibold text-surface hover:bg-brand-strong"
      >
        {destinationLabel}
      </Link>
    </div>
  {:else}
    <div class="flex flex-col items-center gap-4 py-4 text-center" role="alert">
      <span
        class="grid h-12 w-12 place-items-center rounded-full bg-negative-soft text-negative"
        aria-hidden="true"
      >
        <svg viewBox="0 0 24 24" class="h-6 w-6" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 8v5" stroke-linecap="round" />
          <path d="M12 16h.01" stroke-linecap="round" />
          <circle cx="12" cy="12" r="9" />
        </svg>
      </span>
      <div>
        <h2 class="text-base font-semibold text-ink">Verification failed</h2>
        <p class="mt-1 text-sm text-muted">{errorMessage}</p>
      </div>
      <Link
        href={destination}
        class="mt-2 inline-flex min-h-11 items-center justify-center rounded-md border border-control px-4 py-2 text-sm font-semibold text-ink hover:bg-canvas"
      >
        {destinationLabel}
      </Link>
    </div>
  {/if}

  {#snippet footer()}
    Need help? Sign in and request a new verification email.
  {/snippet}
</AuthLayout>
