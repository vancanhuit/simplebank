<script lang="ts">
  import type { Snippet } from "svelte";

  interface Props {
    variant?: "error" | "success" | "info";
    children: Snippet;
  }

  let { variant = "info", children }: Props = $props();

  const styles = {
    error: "bg-negative-soft text-negative",
    success: "bg-positive-soft text-positive",
    info: "bg-brand-soft text-brand-strong",
  };

  // Errors are assertive so screen readers announce them immediately; info and
  // success are polite so they do not interrupt.
  const role = $derived(variant === "error" ? "alert" : "status");
</script>

<div {role} class={`rounded-md px-4 py-3 text-sm font-medium ${styles[variant]}`}>
  {@render children()}
</div>
