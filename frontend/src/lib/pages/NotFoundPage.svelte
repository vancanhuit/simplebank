<script lang="ts">
  import Link from "../components/Link.svelte";
  import { auth } from "../stores/auth.svelte";

  const home = $derived(auth.isAuthenticated ? "/" : "/login");
</script>

<div class="mx-auto max-w-md py-16 text-center">
  <p class="text-sm font-semibold text-brand">404</p>
  <h1 class="mt-2 text-2xl font-semibold text-ink">Page not found</h1>
  <p class="mt-2 text-sm text-muted">The page you're looking for doesn't exist or has moved.</p>
  <div class="mt-6">
    <Link
      href={home}
      class="inline-flex min-h-11 items-center justify-center rounded-md bg-brand px-4 py-2.5 text-sm font-semibold text-surface transition-colors hover:bg-brand-strong"
    >
      Back to safety
    </Link>
  </div>
</div>
