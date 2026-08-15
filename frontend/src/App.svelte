<script lang="ts">
  import { onMount, tick } from "svelte";
  import { auth } from "./lib/stores/auth.svelte";
  import { navigate, router } from "./lib/router.svelte";
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

  onMount(() => auth.init());

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
    if (auth.initializing) {
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

<!-- Persistent live region: announces the page name to screen readers on every
     in-app navigation. It must live outside the view branches so it stays in
     the DOM and only its text changes. -->
<div aria-live="polite" aria-atomic="true" class="sr-only">{routeAnnouncement}</div>

{#if auth.initializing}
  <div class="grid min-h-screen place-items-center bg-canvas" aria-busy="true">
    <span
      class="h-8 w-8 animate-spin rounded-full border-2 border-brand border-t-transparent"
      aria-label="Loading"
    ></span>
  </div>
{:else}
  <div class="flex min-h-screen flex-col">
    {#if view.chrome}
      <a
        href="#main"
        class="sr-only min-h-11 rounded-md bg-brand px-4 py-2 text-surface focus:not-sr-only focus:absolute focus:top-3 focus:left-3 focus:z-10 focus:inline-flex focus:items-center"
      >
        Skip to content
      </a>

      <AppHeader />

      <main id="main" class="mx-auto w-full max-w-6xl flex-1 px-4 py-8 sm:px-6">
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
