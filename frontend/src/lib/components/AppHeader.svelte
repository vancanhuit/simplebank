<script lang="ts">
  import { auth } from "../stores/auth.svelte";
  import { accounts } from "../stores/accounts.svelte";
  import { router } from "../router.svelte";
  import BrandMark from "./BrandMark.svelte";
  import Link from "./Link.svelte";
  import ThemeToggle from "./ThemeToggle.svelte";
  import Menu from "@lucide/svelte/icons/menu";
  import X from "@lucide/svelte/icons/x";
  import LogOut from "@lucide/svelte/icons/log-out";

  const userName = $derived(auth.user?.full_name ?? "");

  const nav = [
    { label: "Overview", href: "/" },
    { label: "Transfer", href: "/transfer" },
  ];

  let menuOpen = $state(false);
  let menuButton: HTMLButtonElement;
  let signingOut = $state(false);
  let logoutError = $state("");

  function toggleMenu() {
    menuOpen = !menuOpen;
  }

  function closeMenu() {
    menuOpen = false;
  }

  function closeMenuAndRestoreFocus() {
    closeMenu();
    menuButton.focus();
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === "Escape" && menuOpen) {
      closeMenu();
      menuButton.focus();
    }
  }

  $effect(() => {
    if (router.path) {
      closeMenu();
    }
  });

  async function logout() {
    signingOut = true;
    logoutError = "";
    try {
      await auth.logout();
      accounts.reset();
    } catch {
      accounts.reset();
      logoutError = "Sign out failed. Check your connection and try again.";
    } finally {
      signingOut = false;
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<header class="border-b border-base-300 bg-base-100">
  <div class="navbar mx-auto min-h-16 max-w-7xl gap-2 px-4 sm:px-6">
    <div class="navbar-start w-auto flex-none gap-1 sm:w-1/2 sm:flex-1 sm:gap-2">
      <button
        bind:this={menuButton}
        type="button"
        class="btn btn-ghost btn-square min-h-11 min-w-11 sm:hidden"
        aria-label={menuOpen ? "Close navigation" : "Open navigation"}
        aria-controls="mobile-primary-navigation"
        aria-expanded={menuOpen}
        onclick={toggleMenu}
      >
        {#if menuOpen}
          <X aria-hidden="true" size={20} />
        {:else}
          <Menu aria-hidden="true" size={20} />
        {/if}
      </button>
      <Link href="/" class="btn btn-ghost h-auto min-h-11 min-w-11 px-1 text-lg">
        <span aria-hidden="true" class="sm:hidden"><BrandMark compact /></span>
        <span aria-hidden="true" class="hidden sm:inline-flex"><BrandMark /></span>
        <span class="sr-only">SimpleBank</span>
      </Link>
    </div>

    <nav aria-label="Primary" class="navbar-center hidden sm:flex">
      <ul class="menu menu-horizontal gap-1 p-0">
        {#each nav as item (item.href)}
          <li>
            <Link
              href={item.href}
              class="min-h-11 rounded-field border-2 border-transparent aria-[current=page]:bg-primary aria-[current=page]:text-primary-content forced-colors:aria-[current=page]:border-[Highlight]"
            >
              {item.label}
            </Link>
          </li>
        {/each}
      </ul>
    </nav>

    <div class="navbar-end ml-auto w-auto flex-none gap-1 sm:w-1/2 sm:flex-1 sm:gap-2">
      <span
        class="hidden min-w-0 max-w-48 truncate text-sm font-medium md:block"
        title={auth.user?.email}
      >
        {userName}
      </span>
      <ThemeToggle />
      <button
        type="button"
        class="btn btn-ghost min-h-11 min-w-11 whitespace-nowrap px-0 sm:px-4"
        onclick={logout}
        disabled={signingOut}
      >
        <LogOut aria-hidden="true" size={16} />
        <span class="sr-only sm:not-sr-only">{signingOut ? "Signing out…" : "Sign out"}</span>
      </button>
    </div>
  </div>
  {#if logoutError}
    <p role="alert" class="mx-auto max-w-7xl px-4 pb-3 text-sm text-error sm:px-6">
      {logoutError}
    </p>
  {/if}

  {#if menuOpen}
    <nav
      id="mobile-primary-navigation"
      aria-label="Mobile primary"
      class="border-t border-base-300 px-4 py-3 sm:hidden"
    >
      <ul class="menu w-full gap-1 p-0">
        {#each nav as item (item.href)}
          <li>
            <Link
              href={item.href}
              class="min-h-11 border-2 border-transparent aria-[current=page]:bg-primary aria-[current=page]:text-primary-content forced-colors:aria-[current=page]:border-[Highlight]"
              onclick={closeMenuAndRestoreFocus}
            >
              {item.label}
            </Link>
          </li>
        {/each}
      </ul>
    </nav>
  {/if}
</header>
