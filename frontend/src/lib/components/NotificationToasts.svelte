<script lang="ts">
  import { formatSignedMoney } from "../money";
  import { notifications } from "../stores/notifications.svelte";
</script>

<div class="toast toast-top toast-end z-50" aria-live="polite" aria-atomic="false">
  {#each notifications.toasts as toast (toast.id)}
    {@const item = toast.notification}
    {@const amount = formatSignedMoney(
      item.direction === "sent" ? -item.amount : item.amount,
      item.currency,
    )}
    <div class={["alert shadow-lg", item.direction === "sent" ? "alert-error" : "alert-success"]}>
      <span>
        {item.direction === "sent" ? "Sent" : "Received"}
        {amount}
        {item.currency}
      </span>
    </div>
  {/each}
</div>
