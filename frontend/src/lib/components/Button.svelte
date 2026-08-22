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

  const variants = {
    primary: "btn-primary",
    secondary: "btn-outline",
    ghost: "btn-ghost",
  };
</script>

<button
  {type}
  class={`btn min-h-11 ${variants[variant]} ${className}`}
  disabled={disabled || loading}
  aria-busy={loading}
  {onclick}
>
  {#if loading}
    <span class="loading loading-spinner loading-sm" aria-hidden="true"></span>
  {/if}
  {@render children()}
</button>
