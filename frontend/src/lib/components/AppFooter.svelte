<script lang="ts">
  // Build identifier baked in at compile time (see vite.config `define`), shown
  // so a running instance can be traced back to its build.
  const appVersion = __APP_VERSION__;

  // The stack this reference app is built on, linked to each project's home.
  // External links, so they open in a new tab without access to this window.
  const stack = [
    { name: "Go", href: "https://go.dev" },
    { name: "Bun", href: "https://bun.sh" },
    { name: "Svelte", href: "https://svelte.dev" },
    { name: "Vite", href: "https://vite.dev" },
    { name: "Tailwind CSS", href: "https://tailwindcss.com" },
    { name: "daisyUI", href: "https://daisyui.com" },
  ];
</script>

<footer
  class="footer footer-vertical border-t border-base-300 bg-base-100 px-4 py-6 text-sm text-base-content/65 sm:footer-horizontal sm:px-6"
>
  <p>© 2026 SimpleBank.</p>

  <p class="flex flex-wrap items-center gap-x-1.5 gap-y-1 sm:justify-self-center">
    <span>Built with</span>
    {#each stack as tech, i (tech.name)}
      <a
        href={tech.href}
        target="_blank"
        rel="noopener noreferrer"
        class="link link-hover font-medium text-base-content"
      >
        {tech.name}
      </a>
      {#if i < stack.length - 1}
        <span aria-hidden="true" class="text-base-content/30">·</span>
      {/if}
    {/each}
  </p>

  <p class="font-mono text-xs sm:justify-self-end">version {appVersion}</p>
</footer>
