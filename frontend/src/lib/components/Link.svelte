<script lang="ts">
  import type { Snippet } from "svelte";
  import { navigate, router } from "../router.svelte";

  interface Props {
    href: string;
    class?: string;
    onclick?: (event: MouseEvent) => void;
    children: Snippet;
  }

  let { href, class: className = "", onclick, children }: Props = $props();

  const isCurrent = $derived(router.path === href);

  function handleClick(event: MouseEvent) {
    onclick?.(event);
    // Let the browser handle cancelled, new-tab, modified, and external navigation.
    if (
      event.defaultPrevented ||
      event.metaKey ||
      event.ctrlKey ||
      event.shiftKey ||
      event.altKey ||
      event.button !== 0 ||
      new URL(href, window.location.href).origin !== window.location.origin
    ) {
      return;
    }
    event.preventDefault();
    navigate(href);
  }
</script>

<a {href} class={className} aria-current={isCurrent ? "page" : undefined} onclick={handleClick}>
  {@render children()}
</a>
