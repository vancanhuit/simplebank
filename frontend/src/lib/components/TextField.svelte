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
    oninput?: (event: Event) => void;
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
    oninput,
  }: Props = $props();

  const hintId = $derived(hint ? `${id}-hint` : undefined);
  const errorId = $derived(error ? `${id}-error` : undefined);
  // Point aria-describedby at the error first (announced before the hint) and
  // fall back to whichever ids exist.
  const describedBy = $derived([errorId, hintId].filter(Boolean).join(" ") || undefined);

  let inputElement: HTMLInputElement;
  let hadError = false;

  $effect(() => {
    if (error && !hadError) {
      inputElement?.focus();
    }
    hadError = Boolean(error);
  });
</script>

<div class="flex flex-col gap-1.5">
  <label for={id} class="text-sm font-medium text-ink contrast-more:font-bold">{label}</label>
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
    {oninput}
    bind:value
    bind:this={inputElement}
    aria-describedby={describedBy}
    aria-invalid={error ? true : undefined}
    class="min-h-11 rounded-md border bg-surface px-3 py-2.5 text-sm text-ink placeholder:text-muted focus-visible:border-brand disabled:bg-surface-muted disabled:cursor-not-allowed contrast-more:border-2 forced-colors:border-[ButtonBorder] {error
      ? 'border-negative contrast-more:border-[Mark] forced-colors:aria-invalid:border-[Mark]'
      : 'border-control'}"
  />
  {#if error}
    <p id={errorId} role="alert" class="text-xs font-medium text-negative contrast-more:font-bold">
      {error}
    </p>
  {/if}
  {#if hint}
    <p id={hintId} class="text-xs text-muted">{hint}</p>
  {/if}
</div>
