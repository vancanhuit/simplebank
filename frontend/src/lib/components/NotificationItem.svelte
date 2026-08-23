<script lang="ts">
  import type { Notification } from "../api/types";
  import { formatSignedMoney } from "../money";

  interface Props {
    notification: Notification;
    compact?: boolean;
    disabled?: boolean;
    onactivate: (notification: Notification) => void | Promise<void>;
  }

  let { notification, compact = false, disabled = false, onactivate }: Props = $props();

  const signedAmount = $derived(
    formatSignedMoney(
      notification.direction === "sent" ? -notification.amount : notification.amount,
      notification.currency,
    ),
  );
  const direction = $derived(notification.direction === "sent" ? "Sent" : "Received");
  const time = $derived(
    new Date(notification.created_at).toLocaleString(undefined, {
      dateStyle: "medium",
      timeStyle: "short",
    }),
  );
  const unread = $derived(notification.read_at === null);
</script>

<button
  type="button"
  class={[
    "list-row min-h-11 w-full cursor-pointer items-center text-left",
    compact ? "gap-2 p-3" : "gap-3 p-4",
    unread && "font-semibold",
  ]}
  aria-label={`${direction}, ${signedAmount}, ${notification.currency}, ${time}${unread ? ", unread" : ""}`}
  {disabled}
  onclick={() => onactivate(notification)}
>
  <span class="list-col-grow min-w-0">
    <span class="block">{direction}</span>
    <span class="block truncate text-xs font-normal text-base-content/70">{time}</span>
  </span>
  <span
    class={[
      "whitespace-nowrap tabular-nums",
      notification.direction === "sent" ? "text-error" : "text-success",
    ]}
  >
    {signedAmount}
  </span>
  {#if unread}
    <span class="badge badge-ghost badge-xs">Unread</span>
  {/if}
</button>
