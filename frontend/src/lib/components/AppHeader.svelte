<script lang="ts">
  import { auth } from "../stores/auth.svelte";
  import { accounts } from "../stores/accounts.svelte";
  import { router } from "../router.svelte";
  import Link from "./Link.svelte";
  import Menu from "@lucide/svelte/icons/menu";
  import X from "@lucide/svelte/icons/x";
  import LogOut from "@lucide/svelte/icons/log-out";

  const userName = $derived(auth.user?.full_name ?? "");

  const initials = $derived(
    userName
      .split(" ")
      .map((part) => part[0] ?? "")
      .slice(0, 2)
      .join("")
      .toUpperCase(),
  );

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

<header class="border-b border-border bg-surface">
  <div class="mx-auto flex h-16 max-w-6xl items-center gap-2 px-4 sm:gap-6 sm:px-6">
    <Link href="/" class="flex min-h-11 shrink-0 items-center gap-2 font-semibold text-ink">
      <span
        class="grid h-8 w-8 place-items-center rounded-lg bg-brand text-surface"
        aria-hidden="true"
      >
        <svg viewBox="0 0 24 24" class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M3 10 12 4l9 6" stroke-linecap="round" stroke-linejoin="round" />
          <path d="M5 10v8h14v-8" stroke-linecap="round" stroke-linejoin="round" />
          <path d="M9 18v-5h6v5" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
      </span>
      SimpleBank
    </Link>

    <nav aria-label="Primary" class="hidden sm:block">
      <ul class="flex items-center gap-1">
        {#each nav as item (item.href)}
          <li>
            <Link
              href={item.href}
              class="flex min-h-11 items-center rounded-md border-l-2 border-transparent px-3 py-2 text-sm font-medium text-muted transition-colors hover:bg-surface-muted hover:text-ink aria-[current=page]:bg-brand-soft aria-[current=page]:text-brand-strong aria-[current=page]:border-brand forced-colors:aria-[current=page]:border-[Highlight]"
            >
              {item.label}
            </Link>
          </li>
        {/each}
      </ul>
    </nav>

    <div class="ml-auto flex min-w-0 items-center gap-2 sm:gap-3">
      <span
        class="hidden min-w-0 max-w-48 items-center gap-2 text-sm font-medium text-ink md:flex"
        title={auth.user?.email}
      >
        <span
          class="grid h-7 w-7 place-items-center rounded-full bg-brand-soft text-xs font-semibold text-brand-strong"
          aria-hidden="true"
        >
          {initials}
        </span>
        <span class="truncate">{userName}</span>
      </span>
      <button
        bind:this={menuButton}
        type="button"
        class="grid min-h-11 min-w-11 place-items-center rounded-md border border-control text-ink transition-colors hover:bg-surface-muted sm:hidden"
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
      <button
        type="button"
        class="flex min-h-11 items-center gap-1.5 whitespace-nowrap rounded-md border border-control px-3 py-2 text-sm font-medium text-ink transition-colors hover:bg-surface-muted"
        onclick={logout}
        disabled={signingOut}
      >
        <LogOut aria-hidden="true" size={16} />
        {signingOut ? "Signing out…" : "Sign out"}
      </button>
    </div>
  </div>
  {#if logoutError}
    <p role="alert" class="mx-auto max-w-6xl px-4 pb-2 text-sm text-negative sm:px-6">
      {logoutError}
    </p>
  {/if}

  {#if menuOpen}
    <nav
      id="mobile-primary-navigation"
      aria-label="Mobile primary"
      class="border-t border-border px-4 py-2 sm:hidden"
    >
      <ul class="mx-auto flex max-w-6xl flex-col">
        {#each nav as item (item.href)}
          <li>
            <Link
              href={item.href}
              class="flex min-h-11 items-center rounded-md border-l-2 border-transparent px-3 text-sm font-medium text-muted hover:bg-surface-muted hover:text-ink aria-[current=page]:bg-brand-soft aria-[current=page]:text-brand-strong aria-[current=page]:border-brand forced-colors:aria-[current=page]:border-[Highlight]"
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
