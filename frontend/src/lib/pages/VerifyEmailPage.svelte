<script lang="ts">
  import { onMount } from "svelte";
  import CircleCheck from "@lucide/svelte/icons/circle-check";
  import CircleAlert from "@lucide/svelte/icons/circle-alert";
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

    // Keep the one-time credential out of browser history after capturing it.
    window.history.replaceState(null, "", window.location.pathname);

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
    <div class="flex flex-col items-center gap-4 py-4 text-center" role="alert">
      <span
        class="grid h-12 w-12 place-items-center rounded-full bg-error/15 text-error"
        aria-hidden="true"
      >
        <CircleAlert class="h-6 w-6" aria-hidden="true" />
      </span>
      <div>
        <h2 class="text-base font-semibold">Verification failed</h2>
        <p class="mt-1 text-sm text-base-content/65">{errorMessage}</p>
      </div>
      <Link href={destination} class="btn btn-outline mt-2">
        {destinationLabel}
      </Link>
    </div>
  {/if}

  {#snippet footer()}
    Need help? Sign in and request a new verification email.
  {/snippet}
</AuthLayout>
