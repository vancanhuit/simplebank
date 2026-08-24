<script lang="ts">
  import { onDestroy, onMount, tick } from "svelte";
  import { auth, type RefreshOutcome } from "./lib/stores/auth.svelte";
  import { accounts } from "./lib/stores/accounts.svelte";
  import { replaceNavigation, router, safeReturnPath } from "./lib/router.svelte";
  import AppHeader from "./lib/components/AppHeader.svelte";
  import AppFooter from "./lib/components/AppFooter.svelte";
  import LoginPage from "./lib/pages/LoginPage.svelte";
  import RegisterPage from "./lib/pages/RegisterPage.svelte";
  import DashboardPage from "./lib/pages/DashboardPage.svelte";
  import TransferPage from "./lib/pages/TransferPage.svelte";
  import NewAccountPage from "./lib/pages/NewAccountPage.svelte";
  import NotFoundPage from "./lib/pages/NotFoundPage.svelte";
  import VerifyEmailPage from "./lib/pages/VerifyEmailPage.svelte";
  import AccountHistoryPage from "./lib/pages/AccountHistoryPage.svelte";
  import NotificationsPage from "./lib/pages/NotificationsPage.svelte";
  import NotificationToasts from "./lib/components/NotificationToasts.svelte";
  import { notifications } from "./lib/stores/notifications.svelte";
  import Alert from "./lib/components/Alert.svelte";
  import Button from "./lib/components/Button.svelte";

  onMount(() => auth.init());
  onDestroy(() => notifications.reset());

  let notificationAuthGeneration: number | null = null;
  $effect(() => {
    if (!auth.initializing && auth.isAuthenticated) {
      if (notificationAuthGeneration !== auth.generation) {
        notificationAuthGeneration = auth.generation;
        notifications.start();
      }
    } else if (!auth.initializing && !auth.renewalUnavailable) {
      notificationAuthGeneration = null;
      notifications.reset();
      accounts.reset();
    }
  });

  let retryingRefresh = $state(false);
  async function retryRefresh(): Promise<void> {
    retryingRefresh = true;
    try {
      const outcome: RefreshOutcome = await auth.retryRefresh();
      if (outcome === "refreshed") {
        await Promise.all([accounts.load(), notifications.reconcile("manual")]);
      }
    } finally {
      retryingRefresh = false;
    }
  }

  // Resolve the view from the path and auth state. Folding the auth guard into
  // resolution means protected pages never render for a signed-out visitor,
  // even for the frame before the URL-sync effect runs. The `label` names the
  // page for the document title and the route announcer.
  const view = $derived.by(() => {
    const path = router.path;
    // Email verification is reachable in any auth state: users may click the
    // link while signed out, signed in, or in another session entirely.
    if (path === "/verify-email") {
      return { component: VerifyEmailPage, chrome: false, label: "Email verification" };
    }
    if (!auth.isAuthenticated) {
      return path === "/register"
        ? { component: RegisterPage, chrome: false, label: "Create account" }
        : { component: LoginPage, chrome: false, label: "Sign in" };
    }
    switch (path) {
      case "/":
        return { component: DashboardPage, chrome: true, label: "Dashboard" };
      case "/transfer":
        return { component: TransferPage, chrome: true, label: "Send money" };
      case "/accounts/new":
        return { component: NewAccountPage, chrome: true, label: "New account" };
      case "/notifications":
        return { component: NotificationsPage, chrome: true, label: "Notifications" };
      default:
        // /accounts/:id (any id other than the reserved "new") shows account
        // activity; everything else is a 404.
        if (/^\/accounts\/[^/]+$/.test(path)) {
          return { component: AccountHistoryPage, chrome: true, label: "Account activity" };
        }
        return { component: NotFoundPage, chrome: true, label: "Page not found" };
    }
  });

  // Announce the resolved page to assistive tech and reflect it in the tab
  // title. A single-page app swaps views without a document load, so without
  // this a screen reader never learns the page changed.
  let routeAnnouncement = $state("");
  let hasNavigated = false;
  $effect(() => {
    // Depend on the resolved label; skip while auth is still resolving so we
    // don't announce a transient loading state.
    const label = view.label;
    if (auth.initializing || (!auth.isAuthenticated && auth.renewalUnavailable)) {
      return;
    }
    document.title = `${label} · SimpleBank`;
    // The initial page load is already announced by the browser and landmarks,
    // so only manage focus and announce on subsequent in-app navigations.
    if (!hasNavigated) {
      hasNavigated = true;
      return;
    }
    routeAnnouncement = label;
    // After the new view renders, move focus to its main region so keyboard
    // users continue from the new content instead of a stale element.
    void tick().then(() => {
      const main = document.querySelector("main");
      if (main instanceof HTMLElement) {
        main.setAttribute("tabindex", "-1");
        main.focus({ preventScroll: false });
      }
    });
  });

  // Keep the address bar in sync with the resolved view so refreshes and shared
  // links land on the right place.
  $effect(() => {
    if (auth.initializing || auth.renewalUnavailable) {
      return;
    }
    const path = router.path;
    // Public pages render regardless of auth and must not be redirected away.
    const isPublic = path === "/login" || path === "/register" || path === "/verify-email";
    if (!auth.isAuthenticated && !isPublic) {
      const state: { returnTo?: string; sessionExpired?: true } = {};
      const returnTo = safeReturnPath(`${path}${window.location.search}`);
      if (returnTo !== null) {
        state.returnTo = returnTo;
      }
      if (auth.consumeSessionExpired()) {
        state.sessionExpired = true;
      }
      replaceNavigation("/login", state);
    } else if (auth.isAuthenticated && (path === "/login" || path === "/register")) {
      replaceNavigation("/");
    }
  });

  const Page = $derived(view.component);
</script>

<!-- Persistent live region: announces the page name to screen readers on every
     in-app navigation. It must live outside the view branches so it stays in
     the DOM and only its text changes. -->
<div aria-live="polite" aria-atomic="true" class="sr-only">{routeAnnouncement}</div>

{#if auth.initializing}
  <div class="grid min-h-screen place-items-center bg-base-200" aria-busy="true">
    <span class="loading loading-ring loading-lg text-primary" aria-label="Loading"></span>
  </div>
{:else if !auth.isAuthenticated && auth.renewalUnavailable}
  <main class="grid min-h-screen place-items-center bg-base-200 px-4 py-10">
    <section class="card card-border w-full max-w-md bg-base-100 shadow-sm">
      <div class="card-body items-start">
        <h1 class="card-title">We couldn't restore your session.</h1>
        <p class="text-base-content/70">Check your connection and try again.</p>
        <div class="card-actions mt-2">
          <Button loading={retryingRefresh} onclick={retryRefresh}>Retry</Button>
        </div>
      </div>
    </section>
  </main>
{:else}
  <div class="flex min-h-screen flex-col bg-base-200 text-base-content">
    {#if view.chrome}
      <a
        href="#main"
        class="btn btn-primary sr-only focus:not-sr-only focus:fixed focus:top-3 focus:left-3 focus:z-50"
      >
        Skip to content
      </a>

      <AppHeader />
      {#if auth.renewalUnavailable}
        <div class="mx-auto w-full max-w-7xl px-4 pt-4 sm:px-6">
          <Alert variant="error">
            <div
              class="flex flex-col items-start gap-3 sm:flex-row sm:items-center sm:justify-between"
            >
              <span>We couldn't restore your session.</span>
              <Button
                variant="secondary"
                loading={retryingRefresh}
                onclick={retryRefresh}
                class="shrink-0">Retry</Button
              >
            </div>
          </Alert>
        </div>
      {/if}
      <NotificationToasts />

      <main id="main" class="mx-auto w-full max-w-7xl flex-1 px-4 py-8 sm:px-6 lg:py-12">
        <Page />
      </main>
    {:else}
      <!-- Public pages (auth, email verification) render their own <main>,
           which grows to fill the shell so the footer stays at the bottom. -->
      <Page />
    {/if}

    <AppFooter />
  </div>
{/if}
