<script lang="ts">
  import type { Component } from "svelte";
  import App from "./App.svelte";
  import AppErrorFallback from "./lib/components/AppErrorFallback.svelte";

  let { content: Content = App }: { content?: Component } = $props();

  function reportError(error: unknown): void {
    if (import.meta.env.DEV) {
      console.error(error);
    }
  }
</script>

<svelte:boundary onerror={reportError}>
  <Content />
  {#snippet failed(_error, reset)}
    <AppErrorFallback {reset} />
  {/snippet}
</svelte:boundary>
