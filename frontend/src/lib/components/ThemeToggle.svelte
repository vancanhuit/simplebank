<script lang="ts">
  import Moon from "@lucide/svelte/icons/moon";
  import Sun from "@lucide/svelte/icons/sun";
  import {
    DARK_THEME,
    LIGHT_THEME,
    applyTheme,
    saveTheme,
    toggleTheme,
    type ThemeName,
  } from "../theme";

  let current = $state<ThemeName>(
    document.documentElement.dataset.theme === DARK_THEME ? DARK_THEME : LIGHT_THEME,
  );
  const nextLabel = $derived(current === LIGHT_THEME ? "dark" : "light");

  function changeTheme() {
    current = toggleTheme(current);
    applyTheme(current);
    saveTheme(current);
  }
</script>

<button
  type="button"
  class="btn btn-ghost btn-square min-h-11 min-w-11"
  aria-label={`Switch to ${nextLabel} theme`}
  onclick={changeTheme}
>
  {#if current === LIGHT_THEME}
    <Moon aria-hidden="true" size={19} />
  {:else}
    <Sun aria-hidden="true" size={19} />
  {/if}
</button>
