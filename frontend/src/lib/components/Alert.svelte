<script lang="ts">
  import type { Snippet } from "svelte";

  interface Props {
    variant?: "error" | "success" | "info";
    children: Snippet;
  }

  let { variant = "info", children }: Props = $props();

  const styles = {
    error: "alert-error",
    success: "alert-success",
    info: "alert-info",
  };

  // Errors are assertive so screen readers announce them immediately; info and
  // success are polite so they do not interrupt.
  const role = $derived(variant === "error" ? "alert" : "status");
</script>

<div
  {role}
  class={`alert ${styles[variant]} text-sm forced-colors:border-[CanvasText] forced-colors:bg-[Canvas] forced-colors:text-[CanvasText]`}
>
  <div>{@render children()}</div>
</div>
