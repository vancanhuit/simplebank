<script lang="ts">
  import type { Notification } from "../api/types";
  import NotificationItem from "../components/NotificationItem.svelte";
  import { navigate } from "../router.svelte";
  import { notifications } from "../stores/notifications.svelte";

  let mutationPending = $state(false);
  let mutationError = $state<string | null>(null);

  async function activate(notification: Notification) {
    if (notification.read_at === null) {
      mutationPending = true;
      mutationError = null;
      try {
        await notifications.markRead(notification.id);
      } catch (cause) {
        mutationError = cause instanceof Error ? cause.message : "Request failed";
        mutationPending = false;
        return;
      }
      mutationPending = false;
    }
    navigate(`/accounts/${notification.account_id}`);
  }

  async function markAllRead() {
    mutationPending = true;
    mutationError = null;
    try {
      await notifications.markAllRead();
    } catch (cause) {
      mutationError = cause instanceof Error ? cause.message : "Request failed";
    } finally {
      mutationPending = false;
    }
  }
</script>

<div class="mx-auto max-w-3xl">
  <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
    <h1 class="text-3xl font-bold tracking-tight text-base-content sm:text-4xl">Notifications</h1>
    <button
      type="button"
      class="btn min-h-11"
      disabled={mutationPending || notifications.unreadCount === 0}
      onclick={markAllRead}
    >
      Mark all read
    </button>
  </div>

  {#if mutationError && mutationError !== notifications.error}
    <div role="alert" class="alert alert-error alert-soft mt-6">
      <span>{mutationError}</span>
    </div>
  {/if}

  {#if notifications.error && notifications.items.length > 0}
    <div role="alert" class="alert alert-error alert-soft mt-6 sm:alert-horizontal">
      <span class="min-w-0 flex-1">{notifications.error}</span>
      <button
        type="button"
        class="btn btn-ghost min-h-11"
        disabled={notifications.refreshing}
        onclick={() => void notifications.reconcile("manual")}
      >
        Retry
      </button>
    </div>
  {/if}

  {#if notifications.loading && notifications.items.length === 0}
    <div
      class="mt-6 grid min-h-48 place-items-center"
      aria-busy="true"
      aria-label="Loading notifications"
    >
      <span class="loading loading-ring loading-lg text-primary"></span>
    </div>
  {:else if notifications.error && notifications.items.length === 0}
    <div role="alert" class="alert alert-error alert-soft mt-6 sm:alert-horizontal">
      <span class="min-w-0 flex-1">{notifications.error}</span>
      <button
        type="button"
        class="btn btn-ghost min-h-11"
        disabled={notifications.loading}
        onclick={() => void notifications.reconcile("manual")}
      >
        Retry
      </button>
    </div>
  {:else if notifications.items.length === 0}
    <div class="card mt-6 border border-dashed border-base-300 bg-base-100 text-center">
      <div class="card-body items-center px-6 py-12">
        <h2 class="card-title text-base">No notifications yet</h2>
        <p class="max-w-sm text-sm text-base-content/70">
          Transfer activity notifications will appear here.
        </p>
      </div>
    </div>
  {:else}
    <div
      class="list mt-6 rounded-box border border-base-300 bg-base-100 shadow-sm"
      aria-busy={notifications.refreshing}
    >
      {#each notifications.items as notification (notification.id)}
        <NotificationItem {notification} disabled={mutationPending} onactivate={activate} />
      {/each}
    </div>
  {/if}

  {#if notifications.loadMoreError}
    <div role="alert" class="alert alert-error alert-soft mt-4 sm:alert-horizontal">
      <span class="min-w-0 flex-1">{notifications.loadMoreError}</span>
      <button
        type="button"
        class="btn btn-ghost min-h-11"
        disabled={notifications.loadingMore}
        onclick={() => void notifications.loadMore()}
      >
        Retry
      </button>
    </div>
  {/if}

  {#if notifications.hasMore}
    <div class="mt-6 flex justify-center">
      <button
        type="button"
        class="btn min-h-11"
        disabled={notifications.loadingMore || mutationPending}
        onclick={() => void notifications.loadMore()}
      >
        {notifications.loadingMore ? "Loading…" : "Load more"}
      </button>
    </div>
  {/if}
</div>
