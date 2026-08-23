import { request, requestResponse, toMessage } from "../api/client";
import { consumeEventStream } from "../api/sse";
import type { Notification, NotificationPage, NotificationReadResponse } from "../api/types";
import { accounts } from "./accounts.svelte";
import { auth } from "./auth.svelte";

export type ReconcileReason =
  "initial" | "connected" | "live" | "visibility" | "manual" | "recovery";

export interface NotificationToast {
  id: string;
  notification: Notification;
}

const RECONNECT_DELAYS = [500, 1_000, 2_000, 5_000, 10_000, 30_000] as const;

class NotificationsStore {
  items = $state.raw<Notification[]>([]);
  unreadCount = $state(0);
  loading = $state(false);
  refreshing = $state(false);
  error = $state<string | null>(null);
  loadingMore = $state(false);
  loadMoreError = $state<string | null>(null);
  nextCursor = $state<string | null>(null);
  toasts = $state.raw<NotificationToast[]>([]);

  #generation = 0;
  #sessionAuthGeneration: number | null = null;
  #controller: AbortController | null = null;
  #knownIds = new Set<string>();
  #pendingLive = new Map<string, Notification>();
  #toastTimers = new Map<string, ReturnType<typeof setTimeout>>();
  #reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  #reconcilePromise: Promise<void> | null = null;
  #queuedReason: ReconcileReason | null = null;
  #activityVersions = $state.raw<Record<string, number>>({});

  get recent(): Notification[] {
    return this.items.slice(0, 5);
  }

  get hasMore(): boolean {
    return this.nextCursor !== null;
  }

  start(): void {
    const authGeneration = auth.generation;
    if (
      this.#sessionAuthGeneration === authGeneration &&
      this.#controller !== null &&
      !this.#controller.signal.aborted
    ) {
      return;
    }

    this.reset();
    const generation = this.#generation;
    const controller = new AbortController();
    this.#sessionAuthGeneration = authGeneration;
    this.#controller = controller;

    document.addEventListener("visibilitychange", this.#handleVisibility);
    void this.reconcile("initial");
    void this.#streamLoop(generation, authGeneration, controller.signal);
  }

  reset(): void {
    this.#generation += 1;
    this.#controller?.abort();
    this.#controller = null;
    this.#sessionAuthGeneration = null;
    document.removeEventListener("visibilitychange", this.#handleVisibility);
    if (this.#reconnectTimer !== null) {
      clearTimeout(this.#reconnectTimer);
      this.#reconnectTimer = null;
    }
    for (const timer of this.#toastTimers.values()) {
      clearTimeout(timer);
    }
    this.#toastTimers.clear();
    this.#knownIds.clear();
    this.#pendingLive.clear();
    this.#reconcilePromise = null;
    this.#queuedReason = null;

    this.items = [];
    this.unreadCount = 0;
    this.loading = false;
    this.refreshing = false;
    this.error = null;
    this.loadingMore = false;
    this.loadMoreError = null;
    this.nextCursor = null;
    this.toasts = [];
    this.#activityVersions = {};
  }

  reconcile(reason: ReconcileReason = "manual"): Promise<void> {
    if (this.#reconcilePromise !== null) {
      this.#queuedReason = this.#mergeQueuedReason(this.#queuedReason, reason);
      return this.#reconcilePromise;
    }

    const generation = this.#generation;
    const authGeneration = auth.generation;
    const signal = this.#controller?.signal;
    const reconciliation = this.#runReconciliations(
      reason,
      generation,
      authGeneration,
      signal,
    ).finally(() => {
      if (this.#reconcilePromise === reconciliation) {
        this.#reconcilePromise = null;
        this.#queuedReason = null;
      }
    });
    this.#reconcilePromise = reconciliation;
    return reconciliation;
  }

  async loadMore(): Promise<void> {
    const cursor = this.nextCursor;
    if (cursor === null || this.loadingMore) {
      return;
    }

    const generation = this.#generation;
    const authGeneration = auth.generation;
    const signal = this.#controller?.signal;
    if (!this.#isCurrent(generation, authGeneration)) {
      return;
    }
    this.loadingMore = true;
    this.loadMoreError = null;

    try {
      const page = await request<NotificationPage>(
        `/notifications?size=20&cursor=${encodeURIComponent(cursor)}`,
        { authenticated: true, signal },
      );
      if (!this.#isCurrent(generation, authGeneration)) {
        return;
      }
      const ids = new Set(this.items.map((notification) => notification.id));
      const additions = page.notifications.filter((notification) => !ids.has(notification.id));
      this.items = [...this.items, ...additions];
      this.unreadCount = page.unread_count;
      this.nextCursor = page.next_cursor;
      for (const notification of additions) {
        this.#knownIds.add(notification.id);
      }
    } catch (cause) {
      if (this.#isCurrent(generation, authGeneration) && !signal?.aborted) {
        this.loadMoreError = toMessage(cause);
      }
    } finally {
      if (this.#isCurrent(generation, authGeneration)) {
        this.loadingMore = false;
      }
    }
  }

  async markRead(id: string): Promise<void> {
    const notification = this.items.find((item) => item.id === id);
    if (!notification || notification.read_at !== null) {
      return;
    }

    const generation = this.#generation;
    const authGeneration = auth.generation;
    const signal = this.#controller?.signal;
    const previousItems = this.items;
    const previousCount = this.unreadCount;
    const readAt = new Date().toISOString();
    if (!this.#isCurrent(generation, authGeneration)) {
      return;
    }
    this.items = this.items.map((item) => (item.id === id ? { ...item, read_at: readAt } : item));
    this.unreadCount = Math.max(0, this.unreadCount - 1);
    this.error = null;

    try {
      const result = await request<NotificationReadResponse>(`/notifications/${id}/read`, {
        method: "PUT",
        authenticated: true,
        signal,
      });
      if (this.#isCurrent(generation, authGeneration)) {
        this.items = this.items.map((item) =>
          item.id === id ? { ...item, read_at: readAt } : item,
        );
        this.unreadCount = result.unread_count;
      }
    } catch (cause) {
      if (this.#isCurrent(generation, authGeneration)) {
        this.items = previousItems;
        this.unreadCount = previousCount;
        if (!signal?.aborted) {
          const message = toMessage(cause);
          await this.reconcile("recovery");
          if (this.#isCurrent(generation, authGeneration)) {
            this.error = message;
          }
        }
      }
      throw cause;
    }
  }

  async markAllRead(): Promise<void> {
    if (this.unreadCount === 0) {
      return;
    }

    const generation = this.#generation;
    const authGeneration = auth.generation;
    const signal = this.#controller?.signal;
    const previousItems = this.items;
    const previousCount = this.unreadCount;
    const readAt = new Date().toISOString();
    if (!this.#isCurrent(generation, authGeneration)) {
      return;
    }
    this.items = this.items.map((item) =>
      item.read_at === null ? { ...item, read_at: readAt } : item,
    );
    this.unreadCount = 0;
    this.error = null;

    try {
      const result = await request<NotificationReadResponse>("/notifications/read-all", {
        method: "PUT",
        authenticated: true,
        signal,
      });
      if (this.#isCurrent(generation, authGeneration)) {
        this.items = this.items.map((item) =>
          item.read_at === null ? { ...item, read_at: readAt } : item,
        );
        this.unreadCount = result.unread_count;
      }
    } catch (cause) {
      if (this.#isCurrent(generation, authGeneration)) {
        this.items = previousItems;
        this.unreadCount = previousCount;
        if (!signal?.aborted) {
          const message = toMessage(cause);
          await this.reconcile("recovery");
          if (this.#isCurrent(generation, authGeneration)) {
            this.error = message;
          }
        }
      }
      throw cause;
    }
  }

  dismissToast(id: string): void {
    const timer = this.#toastTimers.get(id);
    if (timer !== undefined) {
      clearTimeout(timer);
      this.#toastTimers.delete(id);
    }
    this.toasts = this.toasts.filter((toast) => toast.id !== id);
  }

  activityVersion(accountId: string): number {
    return this.#activityVersions[accountId] ?? 0;
  }

  #handleVisibility = (): void => {
    if (document.visibilityState === "visible") {
      this.#queueReconcile("visibility");
    }
  };

  async #runReconciliations(
    firstReason: ReconcileReason,
    generation: number,
    authGeneration: number,
    signal?: AbortSignal,
  ): Promise<void> {
    let reason: ReconcileReason | null = firstReason;
    while (reason !== null && this.#isCurrent(generation, authGeneration)) {
      await this.#reconcileOnce(reason, generation, authGeneration, signal);
      if (!this.#isCurrent(generation, authGeneration)) {
        return;
      }
      reason = this.#queuedReason;
      this.#queuedReason = null;
    }
  }

  async #reconcileOnce(
    reason: ReconcileReason,
    generation: number,
    authGeneration: number,
    signal?: AbortSignal,
  ): Promise<void> {
    if (!this.#isCurrent(generation, authGeneration)) {
      return;
    }
    const initial = this.items.length === 0;
    if (initial) {
      this.loading = true;
    } else {
      this.refreshing = true;
    }
    this.error = null;

    try {
      const page = await request<NotificationPage>("/notifications?size=20", {
        authenticated: true,
        signal,
      });
      if (!this.#isCurrent(generation, authGeneration)) {
        return;
      }

      const newlyDiscovered = page.notifications.filter(
        (notification) => !this.#knownIds.has(notification.id),
      );
      this.items = [
        ...page.notifications,
        ...this.items.filter(
          (notification) => !page.notifications.some((current) => current.id === notification.id),
        ),
      ];
      this.unreadCount = page.unread_count;
      this.nextCursor = page.next_cursor;

      if (reason !== "initial" && newlyDiscovered.length > 0) {
        const versions = { ...this.#activityVersions };
        for (const notification of newlyDiscovered) {
          versions[notification.account_id] = (versions[notification.account_id] ?? 0) + 1;
        }
        this.#activityVersions = versions;
      }

      for (const notification of newlyDiscovered) {
        this.#knownIds.add(notification.id);
        if (reason === "live") {
          this.#pendingLive.set(notification.id, notification);
        }
      }

      let accountsApplied = false;
      try {
        accountsApplied = await accounts.load(signal);
      } catch {
        accountsApplied = false;
      }
      if (!this.#isCurrent(generation, authGeneration)) {
        return;
      }
      if (accountsApplied) {
        this.#publishPendingToasts(generation, authGeneration);
      }
    } catch (cause) {
      if (this.#isCurrent(generation, authGeneration) && !signal?.aborted) {
        this.error = toMessage(cause);
      }
    } finally {
      if (this.#isCurrent(generation, authGeneration)) {
        this.loading = false;
        this.refreshing = false;
      }
    }
  }

  #publishPendingToasts(generation: number, authGeneration: number): void {
    if (!this.#isCurrent(generation, authGeneration) || this.#pendingLive.size === 0) {
      return;
    }
    const additions = [...this.#pendingLive.values()].map((notification) => ({
      id: notification.id,
      notification,
    }));
    this.#pendingLive.clear();
    this.toasts = [...this.toasts, ...additions];

    for (const toast of additions) {
      const timer = setTimeout(() => {
        if (this.#isCurrent(generation, authGeneration)) {
          this.dismissToast(toast.id);
        }
      }, 5_000);
      this.#toastTimers.set(toast.id, timer);
    }
  }

  #queueReconcile(reason: ReconcileReason): void {
    void this.reconcile(reason);
  }

  #mergeQueuedReason(current: ReconcileReason | null, next: ReconcileReason): ReconcileReason {
    return current === "live" || next === "live" ? "live" : next;
  }

  async #streamLoop(
    generation: number,
    authGeneration: number,
    signal: AbortSignal,
  ): Promise<void> {
    let reconnectIndex = 0;
    while (this.#isCurrent(generation, authGeneration) && !signal.aborted) {
      try {
        const response = await requestResponse("/notifications/stream", {
          authenticated: true,
          signal,
        });
        if (!this.#isCurrent(generation, authGeneration)) {
          return;
        }
        reconnectIndex = 0;
        await this.reconcile("connected");
        await consumeEventStream(response, () => this.#queueReconcile("live"), signal);
      } catch {
        if (!this.#isCurrent(generation, authGeneration) || signal.aborted) {
          return;
        }
      }

      if (!this.#isCurrent(generation, authGeneration) || signal.aborted) {
        return;
      }

      const delay = RECONNECT_DELAYS[Math.min(reconnectIndex, RECONNECT_DELAYS.length - 1)];
      reconnectIndex = Math.min(reconnectIndex + 1, RECONNECT_DELAYS.length - 1);
      await this.#waitForReconnect(delay, signal);
    }
  }

  #waitForReconnect(delay: number, signal: AbortSignal): Promise<void> {
    if (signal.aborted) {
      return Promise.resolve();
    }
    return new Promise((resolve) => {
      const finish = () => {
        if (this.#reconnectTimer !== null) {
          clearTimeout(this.#reconnectTimer);
          this.#reconnectTimer = null;
        }
        signal.removeEventListener("abort", finish);
        resolve();
      };
      this.#reconnectTimer = setTimeout(finish, delay);
      signal.addEventListener("abort", finish, { once: true });
    });
  }

  #isCurrent(generation: number, authGeneration: number): boolean {
    return this.#generation === generation && auth.generation === authGeneration;
  }
}

export const notifications = new NotificationsStore();
