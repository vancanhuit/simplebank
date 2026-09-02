<script lang="ts">
  import Link from "./Link.svelte";

  let { href }: { href: string } = $props();
</script>

<Link {href}>Test link</Link>
