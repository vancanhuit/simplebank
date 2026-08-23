<script lang="ts">
  import Bell from "@lucide/svelte/icons/bell";
  import type { Attachment } from "svelte/attachments";
  import type { Notification } from "../api/types";
  import { navigate } from "../router.svelte";
  import { notifications } from "../stores/notifications.svelte";
  import Link from "./Link.svelte";
  import NotificationItem from "./NotificationItem.svelte";

  let popover: HTMLElement | undefined;
  let pending = $state(false);
  let operationError = $state<string | null>(null);
  let retryOperation = $state<(() => Promise<void>) | null>(null);

  const visualCount = $derived(notifications.unreadCount > 99 ? "99+" : notifications.unreadCount);

  const capturePopover: Attachment<HTMLElement> = (element) => {
    popover = element;
    return () => {
      if (popover === element) {
        popover = undefined;
      }
    };
  };

  function closePopover() {
    if (popover && typeof popover.hidePopover === "function") {
      popover.hidePopover();
    }
  }

  async function run(operation: () => Promise<void>) {
    pending = true;
    operationError = null;
    retryOperation = operation;
    try {
      await operation();
      retryOperation = null;
    } catch (cause) {
      operationError = cause instanceof Error ? cause.message : "Request failed";
    } finally {
      pending = false;
    }
  }

  async function activate(notification: Notification) {
    if (notification.read_at !== null) {
      closePopover();
      navigate(`/accounts/${notification.account_id}`);
      return;
    }

    await run(async () => {
      await notifications.markRead(notification.id);
      closePopover();
      navigate(`/accounts/${notification.account_id}`);
    });
  }

  async function markAllRead() {
    await run(() => notifications.markAllRead());
  }

  async function retry() {
    if (retryOperation !== null) {
      await run(retryOperation);
    }
  }

  function viewAll() {
    closePopover();
  }
</script>

<button
  type="button"
  class="btn btn-ghost btn-square relative min-h-11 min-w-11"
  aria-label={`Notifications, ${notifications.unreadCount} unread`}
  popovertarget="notification-preview"
  style="anchor-name:--notification-bell"
>
  <Bell aria-hidden="true" size={19} />
  {#if notifications.unreadCount > 0}
    <span
      aria-hidden="true"
      class="badge badge-primary badge-xs absolute -top-0.5 -right-1 max-w-8 px-1 text-[0.625rem]"
    >
      {visualCount}
    </span>
  {/if}
</button>

<section
  {@attach capturePopover}
  id="notification-preview"
  class="dropdown dropdown-end m-0 w-[min(20rem,calc(100vw-1rem))] rounded-box border border-base-300 bg-base-100 p-0 shadow-xl"
  popover
  style="position-anchor:--notification-bell"
  aria-label="Recent notifications"
>
  <div class="flex items-center justify-between gap-2 border-b border-base-300 px-4 py-3">
    <h2 class="font-semibold">Notifications</h2>
    <button
      type="button"
      class="btn btn-ghost btn-sm min-h-11"
      disabled={pending || notifications.unreadCount === 0}
      onclick={markAllRead}
    >
      Mark all read
    </button>
  </div>

  {#if operationError}
    <div class="alert alert-error alert-soft m-3 py-2 text-sm">
      <span class="min-w-0 flex-1">{operationError}</span>
      <button
        type="button"
        class="btn btn-ghost btn-sm min-h-11"
        disabled={pending}
        onclick={retry}
      >
        Retry
      </button>
    </div>
  {/if}

  <div class="list max-h-80 overflow-y-auto">
    {#each notifications.recent as notification (notification.id)}
      <NotificationItem {notification} compact disabled={pending} onactivate={activate} />
    {:else}
      <p class="p-4 text-sm text-base-content/70">No notifications yet.</p>
    {/each}
  </div>

  <div class="border-t border-base-300 p-2">
    <Link href="/notifications" class="btn btn-ghost min-h-11 w-full" onclick={viewAll}>
      View all notifications
    </Link>
  </div>
</section>
