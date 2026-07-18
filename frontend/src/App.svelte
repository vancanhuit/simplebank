<script lang="ts">
  import { onMount } from "svelte";
  import { auth } from "./lib/stores/auth.svelte";
  import { navigate, router } from "./lib/router.svelte";
  import AppHeader from "./lib/components/AppHeader.svelte";
  import LoginPage from "./lib/pages/LoginPage.svelte";
  import RegisterPage from "./lib/pages/RegisterPage.svelte";
  import DashboardPage from "./lib/pages/DashboardPage.svelte";
  import TransferPage from "./lib/pages/TransferPage.svelte";
  import NewAccountPage from "./lib/pages/NewAccountPage.svelte";
  import NotFoundPage from "./lib/pages/NotFoundPage.svelte";
  import VerifyEmailPage from "./lib/pages/VerifyEmailPage.svelte";
  import AccountHistoryPage from "./lib/pages/AccountHistoryPage.svelte";

  onMount(() => auth.init());

  // Resolve the view from the path and auth state. Folding the auth guard into
  // resolution means protected pages never render for a signed-out visitor,
  // even for the frame before the URL-sync effect runs.
  const view = $derived.by(() => {
    const path = router.path;
    // Email verification is reachable in any auth state: users may click the
    // link while signed out, signed in, or in another session entirely.
    if (path === "/verify-email") {
      return { component: VerifyEmailPage, chrome: false };
    }
    if (!auth.isAuthenticated) {
      return path === "/register"
        ? { component: RegisterPage, chrome: false }
        : { component: LoginPage, chrome: false };
    }
    switch (path) {
      case "/":
        return { component: DashboardPage, chrome: true };
      case "/transfer":
        return { component: TransferPage, chrome: true };
      case "/accounts/new":
        return { component: NewAccountPage, chrome: true };
      default:
        // /accounts/:id (any id other than the reserved "new") shows account
        // activity; everything else is a 404.
        if (/^\/accounts\/[^/]+$/.test(path)) {
          return { component: AccountHistoryPage, chrome: true };
        }
        return { component: NotFoundPage, chrome: true };
    }
  });

  // Keep the address bar in sync with the resolved view so refreshes and shared
  // links land on the right place.
  $effect(() => {
    if (auth.initializing) {
      return;
    }
    const path = router.path;
    // Public pages render regardless of auth and must not be redirected away.
    const isPublic = path === "/login" || path === "/register" || path === "/verify-email";
    if (!auth.isAuthenticated && !isPublic) {
      navigate("/login");
    } else if (auth.isAuthenticated && (path === "/login" || path === "/register")) {
      navigate("/");
    }
  });

  const Page = $derived(view.component);
</script>

{#if auth.initializing}
  <div class="grid min-h-screen place-items-center bg-canvas" aria-busy="true">
    <span
      class="h-8 w-8 animate-spin rounded-full border-2 border-brand border-t-transparent"
      aria-label="Loading"
    ></span>
  </div>
{:else if view.chrome}
  <div class="flex min-h-screen flex-col">
    <a
      href="#main"
      class="sr-only rounded-md bg-brand px-4 py-2 text-surface focus:not-sr-only focus:absolute focus:top-3 focus:left-3 focus:z-10"
    >
      Skip to content
    </a>

    <AppHeader />

    <main id="main" class="mx-auto w-full max-w-6xl flex-1 px-4 py-8 sm:px-6">
      <Page />
    </main>

    <footer class="border-t border-border bg-surface">
      <div
        class="mx-auto flex max-w-6xl flex-col gap-1 px-4 py-6 text-sm text-muted sm:flex-row sm:items-center sm:justify-between sm:px-6"
      >
        <p>© 2026 SimpleBank. Demo interface.</p>
        <p>A cloud-native reference application.</p>
      </div>
    </footer>
  </div>
{:else}
  <Page />
{/if}
