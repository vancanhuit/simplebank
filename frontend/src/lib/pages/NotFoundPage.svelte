<script lang="ts">
  import Link from "../components/Link.svelte";
  import { auth } from "../stores/auth.svelte";

  const home = $derived(auth.isAuthenticated ? "/" : "/login");
</script>

<div class="hero py-10 sm:py-16">
  <div class="hero-content w-full max-w-lg">
    <div class="card w-full border border-base-300 bg-base-100 text-center shadow-sm">
      <div class="card-body items-center px-6 py-10 sm:px-10 sm:py-12">
        <p class="text-sm font-bold tracking-[0.25em] text-primary">404</p>
        <h1 class="card-title mt-1 text-3xl tracking-tight">Page not found</h1>
        <p class="mt-1 text-sm text-base-content/60">
          The page you're looking for doesn't exist or has moved.
        </p>
        <div class="card-actions mt-4">
          <Link href={home} class="btn btn-primary min-h-11">Back to safety</Link>
        </div>
      </div>
    </div>
  </div>
</div>
