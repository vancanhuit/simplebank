<script lang="ts">
  import { formatSignedMoney } from "../money";
  import type { ActivityItem } from "../types";

  interface Props {
    items: ActivityItem[];
  }

  let { items }: Props = $props();

  const dateFormatter = new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
</script>

<div class="overflow-hidden rounded-card border border-border bg-surface">
  {#if items.length === 0}
    <div role="status" class="flex flex-col items-center gap-2 px-6 py-12 text-center">
      <svg
        viewBox="0 0 24 24"
        class="h-10 w-10 text-muted"
        fill="none"
        stroke="currentColor"
        stroke-width="1.5"
        aria-hidden="true"
      >
        <rect x="3" y="6" width="18" height="13" rx="2" />
        <path d="M3 10h18" stroke-linecap="round" />
      </svg>
      <h3 class="text-sm font-medium text-ink">No activity yet</h3>
      <p class="max-w-xs text-sm text-muted">
        Transfers and deposits will appear here once your accounts start moving money.
      </p>
    </div>
  {:else}
    <ul role="list" class="divide-y divide-border">
      {#each items as item (item.id)}
        {@const incoming = item.amount >= 0}
        <li class="flex items-center gap-4 px-4 py-3 sm:px-5">
          <span
            class="grid h-9 w-9 shrink-0 place-items-center rounded-full {incoming
              ? 'bg-positive-soft text-positive'
              : 'bg-surface-muted text-muted'}"
            aria-hidden="true"
          >
            <svg
              viewBox="0 0 24 24"
              class="h-4 w-4"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
            >
              {#if incoming}
                <path d="M12 5v14M5 12l7 7 7-7" stroke-linecap="round" stroke-linejoin="round" />
              {:else}
                <path d="M12 19V5M5 12l7-7 7 7" stroke-linecap="round" stroke-linejoin="round" />
              {/if}
            </svg>
          </span>

          <div class="min-w-0 flex-1">
            <p class="truncate text-sm font-medium text-ink">{item.counterparty}</p>
            <p class="text-xs text-muted">
              {incoming ? "Received" : "Sent"} · {dateFormatter.format(new Date(item.occurredAt))}
            </p>
          </div>

          <p
            class="shrink-0 text-sm font-semibold tabular-nums {incoming
              ? 'text-positive'
              : 'text-ink'}"
          >
            {formatSignedMoney(item.amount, item.currency)}
          </p>
        </li>
      {/each}
    </ul>
  {/if}
</div>
