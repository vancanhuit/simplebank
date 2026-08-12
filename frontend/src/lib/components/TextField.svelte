<script lang="ts">
  interface Props {
    /** Optional explicit id. Defaults to a unique, SSR-safe id so callers
        don't have to hand-manage one and fields can repeat without collisions. */
    id?: string;
    label: string;
    value: string;
    type?: "text" | "email" | "password" | "number";
    placeholder?: string;
    autocomplete?: HTMLInputElement["autocomplete"];
    required?: boolean;
    inputmode?: "text" | "numeric" | "decimal" | "email";
    step?: string;
    min?: string;
    max?: string;
    hint?: string;
    error?: string;
    disabled?: boolean;
  }

  const uid = $props.id();

  let {
    id = uid,
    label,
    value = $bindable(),
    type = "text",
    placeholder,
    autocomplete,
    required = false,
    inputmode,
    step,
    min,
    max,
    hint,
    error,
    disabled = false,
  }: Props = $props();

  const hintId = $derived(hint ? `${id}-hint` : undefined);
  const errorId = $derived(error ? `${id}-error` : undefined);
  // Point aria-describedby at the error first (announced before the hint) and
  // fall back to whichever ids exist.
  const describedBy = $derived([errorId, hintId].filter(Boolean).join(" ") || undefined);
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
    {max}
    {disabled}
    bind:value
    aria-describedby={describedBy}
    aria-invalid={error ? true : undefined}
    class="rounded-md border bg-surface px-3 py-2.5 text-sm text-ink placeholder:text-muted focus-visible:border-brand disabled:bg-surface-muted {error
      ? 'border-negative'
      : 'border-border'}"
  />
  {#if error}
    <p id={errorId} class="text-xs font-medium text-negative">{error}</p>
  {/if}
  {#if hint}
    <p id={hintId} class="text-xs text-muted">{hint}</p>
  {/if}
</div>
