<script lang="ts">
  import type { Snippet } from "svelte";
  import BrandMark from "../components/BrandMark.svelte";

  interface Props {
    title: string;
    subtitle: string;
    children: Snippet;
    footer: Snippet;
  }

  let { title, subtitle, children, footer }: Props = $props();
</script>

<main class="grid min-h-[calc(100vh-5rem)] flex-1 lg:grid-cols-[minmax(0,1fr)_minmax(28rem,0.8fr)]">
  <section
    class="hidden bg-neutral px-10 py-14 text-neutral-content lg:flex lg:flex-col lg:justify-between"
    aria-label="SimpleBank introduction"
  >
    <BrandMark />
    <div class="max-w-xl">
      <p class="text-sm font-semibold tracking-[0.18em] uppercase">Clear money movement</p>
      <p class="mt-5 text-5xl leading-[1.05] font-semibold tracking-tight">
        Banking built around what matters.
      </p>
      <p class="mt-5 max-w-md text-base text-neutral-content/70">
        See every balance clearly, move money confidently, and stay in control.
      </p>
    </div>
    <p class="text-sm text-neutral-content/60">Secure by design. Simple by default.</p>
  </section>
  <section class="flex items-center justify-center px-4 py-10 sm:px-8">
    <div class="w-full max-w-md">
      <div class="mb-8 lg:hidden"><BrandMark /></div>
      <div class="card card-border bg-base-100 shadow-sm">
        <div class="card-body gap-0 p-6 sm:p-8">
          <h1 class="card-title text-3xl tracking-tight">{title}</h1>
          <p class="mt-2 text-base-content/65">{subtitle}</p>
          <div class="mt-7">{@render children()}</div>
        </div>
      </div>
      <p class="mt-6 text-center text-sm text-base-content/65">{@render footer()}</p>
    </div>
  </section>
</main>
