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

<fieldset class="fieldset">
  <label for={id} class="fieldset-legend contrast-more:font-bold">{label}</label>
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
    class="input w-full min-h-11 disabled:cursor-not-allowed contrast-more:border-2 forced-colors:border-[ButtonBorder] {error
      ? 'input-error forced-colors:aria-invalid:border-[Mark]'
      : ''}"
  />
  {#if error}
    <p id={errorId} role="alert" class="label text-error contrast-more:font-bold">
      {error}
    </p>
  {/if}
  {#if hint}
    <p id={hintId} class="label text-base-content/70">{hint}</p>
  {/if}
</fieldset>
