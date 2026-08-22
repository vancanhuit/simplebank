<script lang="ts">
  interface Props {
    compact?: boolean;
  }

  let { compact = false }: Props = $props();
</script>

<span class="inline-flex items-center gap-2.5 font-semibold tracking-tight">
  <span
    class="grid size-9 place-items-center rounded-box bg-primary text-primary-content"
    aria-hidden="true"
  >
    <svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="2">
      <path d="M3 10 12 4l9 6" stroke-linecap="round" stroke-linejoin="round" />
      <path d="M5 10v8h14v-8" stroke-linecap="round" stroke-linejoin="round" />
      <path d="M9 18v-5h6v5" stroke-linecap="round" stroke-linejoin="round" />
    </svg>
  </span>
  <span class:sr-only={compact}>SimpleBank</span>
</span>
