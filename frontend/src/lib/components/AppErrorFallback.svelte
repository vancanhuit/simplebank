<script lang="ts">
  import Button from "./Button.svelte";

  interface Props {
    reset: () => void;
    reload?: () => void;
  }

  let { reset, reload = () => window.location.reload() }: Props = $props();
</script>

<main class="grid min-h-screen place-items-center bg-base-200 px-4 py-10">
  <section
    role="alert"
    class="card card-border w-full max-w-md bg-base-100 shadow-sm forced-colors:border-[CanvasText]"
  >
    <div class="card-body items-start">
      <h1 class="card-title">We couldn't display SimpleBank.</h1>
      <p class="text-base-content/70">Try again, or reload the page if the problem continues.</p>
      <div class="card-actions mt-2">
        <Button onclick={reset}>Try again</Button>
        <Button variant="secondary" onclick={reload}>Reload page</Button>
      </div>
    </div>
  </section>
</main>
