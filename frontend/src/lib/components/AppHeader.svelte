<script lang="ts">
  import { auth } from "../stores/auth.svelte";
  import { accounts } from "../stores/accounts.svelte";
  import Link from "./Link.svelte";

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

  function logout() {
    accounts.reset();
    auth.logout();
  }
</script>

<header class="border-b border-border bg-surface">
  <div class="mx-auto flex h-16 max-w-6xl items-center gap-6 px-4 sm:px-6">
    <Link href="/" class="flex items-center gap-2 font-semibold text-ink">
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
              class="block rounded-md px-3 py-2 text-sm font-medium text-muted transition-colors hover:bg-surface-muted hover:text-ink aria-[current=page]:bg-brand-soft aria-[current=page]:text-brand-strong"
            >
              {item.label}
            </Link>
          </li>
        {/each}
      </ul>
    </nav>

    <div class="ml-auto flex items-center gap-3">
      <span
        class="hidden items-center gap-2 text-sm font-medium text-ink sm:flex"
        title={auth.user?.email}
      >
        <span
          class="grid h-7 w-7 place-items-center rounded-full bg-brand-soft text-xs font-semibold text-brand-strong"
          aria-hidden="true"
        >
          {initials}
        </span>
        {userName}
      </span>
      <button
        type="button"
        class="rounded-md border border-border px-3 py-1.5 text-sm font-medium text-ink transition-colors hover:bg-surface-muted"
        onclick={logout}
      >
        Sign out
      </button>
    </div>
  </div>
</header>
