<script module lang="ts">
  let shouldThrow = true;

  export function allowRender() {
    shouldThrow = false;
  }
</script>

<script lang="ts">
  if (shouldThrow) throw new Error("database password leaked");
</script>

<p>Recovered content</p>
