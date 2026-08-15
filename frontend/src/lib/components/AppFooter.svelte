<script lang="ts">
  // Build identifier baked in at compile time (see vite.config `define`), shown
  // so a running instance can be traced back to its build.
  const appVersion = __APP_VERSION__;

  // The stack this reference app is built on, linked to each project's home.
  // External links, so they open in a new tab with rel="noreferrer".
  const stack = [
    { name: "Go", href: "https://go.dev" },
    { name: "Bun", href: "https://bun.sh" },
    { name: "Svelte", href: "https://svelte.dev" },
    { name: "Vite", href: "https://vite.dev" },
    { name: "Tailwind CSS", href: "https://tailwindcss.com" },
  ];
</script>

<footer class="border-t border-border bg-surface">
  <div
    class="mx-auto flex max-w-6xl flex-col gap-2 px-4 py-6 text-sm text-muted sm:flex-row sm:items-center sm:justify-between sm:px-6"
  >
    <p>© 2026 SimpleBank.</p>

    <p class="flex flex-wrap items-center gap-x-1.5 gap-y-1">
      <span>Built with</span>
      {#each stack as tech, i (tech.name)}
        <a
          href={tech.href}
          target="_blank"
          rel="noreferrer"
          class="rounded-sm font-medium text-ink underline-offset-4 hover:text-brand hover:underline"
        >
          {tech.name}
        </a>
        {#if i < stack.length - 1}
          <span aria-hidden="true" class="text-border">·</span>
        {/if}
      {/each}
    </p>

    <p class="font-mono text-xs">version {appVersion}</p>
  </div>
</footer>
