<script lang="ts">
  interface Props {
    id: string;
    label: string;
    value: string;
    type?: "text" | "email" | "password" | "number";
    placeholder?: string;
    autocomplete?: HTMLInputElement["autocomplete"];
    required?: boolean;
    inputmode?: "text" | "numeric" | "decimal" | "email";
    step?: string;
    min?: string;
    hint?: string;
    disabled?: boolean;
  }

  let {
    id,
    label,
    value = $bindable(),
    type = "text",
    placeholder,
    autocomplete,
    required = false,
    inputmode,
    step,
    min,
    hint,
    disabled = false,
  }: Props = $props();

  const hintId = $derived(hint ? `${id}-hint` : undefined);
</script>

<div class="flex flex-col gap-1.5">
  <label for={id} class="text-sm font-medium text-ink">{label}</label>
  <input
    {id}
    {type}
    {placeholder}
    {autocomplete}
    {required}
    {inputmode}
    {step}
    {min}
    {disabled}
    bind:value
    aria-describedby={hintId}
    class="rounded-md border border-border bg-surface px-3 py-2.5 text-sm text-ink placeholder:text-muted focus-visible:border-brand disabled:bg-surface-muted"
  />
  {#if hint}
    <p id={hintId} class="text-xs text-muted">{hint}</p>
  {/if}
</div>
