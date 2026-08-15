<script lang="ts">
  import type { Snippet } from "svelte";

  interface Props {
    variant?: "error" | "success" | "info";
    children: Snippet;
  }

  let { variant = "info", children }: Props = $props();

  const styles = {
    error:
      "border-negative bg-negative-soft text-negative forced-colors:border-[CanvasText] forced-colors:bg-[Canvas] forced-colors:text-[CanvasText]",
    success:
      "border-positive bg-positive-soft text-positive forced-colors:border-[CanvasText] forced-colors:bg-[Canvas] forced-colors:text-[CanvasText]",
    info: "border-info bg-info-soft text-info forced-colors:border-[CanvasText] forced-colors:bg-[Canvas] forced-colors:text-[CanvasText]",
  };

  // Errors are assertive so screen readers announce them immediately; info and
  // success are polite so they do not interrupt.
  const role = $derived(variant === "error" ? "alert" : "status");
</script>

<div {role} class={`rounded-md border px-4 py-3 text-sm font-medium ${styles[variant]}`}>
  {@render children()}
</div>
