<script lang="ts">
  import type { Snippet } from "svelte";

  interface Props {
    title: string;
    subtitle: string;
    children: Snippet;
    footer: Snippet;
  }

  let { title, subtitle, children, footer }: Props = $props();
</script>

<main class="mx-auto flex w-full max-w-md flex-1 flex-col justify-center px-4 py-12">
  <div class="mb-8 flex items-center gap-2 text-lg font-semibold text-ink">
    <span
      class="grid h-9 w-9 place-items-center rounded-lg bg-brand text-surface"
      aria-hidden="true"
    >
      <svg viewBox="0 0 24 24" class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M3 10 12 4l9 6" stroke-linecap="round" stroke-linejoin="round" />
        <path d="M5 10v8h14v-8" stroke-linecap="round" stroke-linejoin="round" />
        <path d="M9 18v-5h6v5" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
    </span>
    SimpleBank
  </div>

  <div class="rounded-card border border-border bg-surface p-6 sm:p-8">
    <h1 class="text-xl font-semibold text-ink">{title}</h1>
    <p class="mt-1 text-sm text-muted">{subtitle}</p>

    <div class="mt-6">
      {@render children()}
    </div>
  </div>

  <p class="mt-6 text-center text-sm text-muted">
    {@render footer()}
  </p>
</main>
