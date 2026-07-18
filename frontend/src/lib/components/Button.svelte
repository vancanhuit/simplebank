<script lang="ts">
  import type { Snippet } from "svelte";

  interface Props {
    type?: "button" | "submit";
    variant?: "primary" | "secondary" | "ghost";
    disabled?: boolean;
    loading?: boolean;
    class?: string;
    onclick?: (event: MouseEvent) => void;
    children: Snippet;
  }

  let {
    type = "button",
    variant = "primary",
    disabled = false,
    loading = false,
    class: className = "",
    onclick,
    children,
  }: Props = $props();

  const base =
    "inline-flex items-center justify-center gap-2 rounded-md px-4 py-2.5 text-sm font-semibold transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 disabled:cursor-not-allowed disabled:opacity-60";

  const variants = {
    primary: "bg-brand text-surface hover:bg-brand-strong",
    secondary: "border border-border bg-surface text-ink hover:bg-surface-muted",
    ghost: "text-brand hover:bg-brand-soft/60",
  };
</script>

<button
  {type}
  class={`${base} ${variants[variant]} ${className}`}
  disabled={disabled || loading}
  aria-busy={loading}
  {onclick}
>
  {#if loading}
    <span
      class="h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent"
      aria-hidden="true"
    ></span>
  {/if}
  {@render children()}
</button>
