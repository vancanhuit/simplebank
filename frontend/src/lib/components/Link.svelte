<script lang="ts">
  import type { Snippet } from "svelte";
  import { navigate, router } from "../router.svelte";

  interface Props {
    href: string;
    class?: string;
    children: Snippet;
  }

  let { href, class: className = "", children }: Props = $props();

  const isCurrent = $derived(router.path === href);

  function handleClick(event: MouseEvent) {
    // Let the browser handle new-tab / modified clicks and external links.
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
      return;
    }
    event.preventDefault();
    navigate(href);
  }
</script>

<a {href} class={className} aria-current={isCurrent ? "page" : undefined} onclick={handleClick}>
  {@render children()}
</a>
