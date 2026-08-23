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

interface SessionContext {
  generation: number;
  authGeneration: number;
  controller: AbortController;
}

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
  #streamStarted = false;
  #visibilityListener: (() => void) | null = null;
  #hasBaseline = false;
  #knownIds = new Set<string>();
  #pendingLive = new Map<string, Notification>();
  #toastTimers = new Map<string, ReturnType<typeof setTimeout>>();
  #reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  #reconcilePromise: Promise<void> | null = null;
  #queuedReason: ReconcileReason | null = null;
  #mutationQueue: Promise<void> = Promise.resolve();
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
      this.#streamStarted &&
      this.#sessionAuthGeneration === authGeneration &&
      this.#controller !== null &&
      !this.#controller.signal.aborted
    ) {
      return;
    }

    if (this.#sessionAuthGeneration !== authGeneration || this.#controller?.signal.aborted) {
      this.reset();
    }
    const context = this.#sessionContext();
    if (context === null) {
      return;
    }
    this.#streamStarted = true;
    this.#visibilityListener = () => {
      if (document.visibilityState === "visible") {
        this.#queueReconcile("visibility", context);
      }
    };

    document.addEventListener("visibilitychange", this.#visibilityListener);
    void this.reconcile("initial");
    void this.#streamLoop(context);
  }

  reset(): void {
    this.#generation += 1;
    this.#controller?.abort();
    this.#controller = null;
    this.#sessionAuthGeneration = null;
    this.#streamStarted = false;
    if (this.#visibilityListener !== null) {
      document.removeEventListener("visibilitychange", this.#visibilityListener);
      this.#visibilityListener = null;
    }
    if (this.#reconnectTimer !== null) {
      clearTimeout(this.#reconnectTimer);
      this.#reconnectTimer = null;
    }
    for (const timer of this.#toastTimers.values()) {
      clearTimeout(timer);
    }
    this.#toastTimers.clear();
    this.#knownIds.clear();
    this.#hasBaseline = false;
    this.#pendingLive.clear();
    this.#reconcilePromise = null;
    this.#queuedReason = null;
    this.#mutationQueue = Promise.resolve();

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
    const context = this.#sessionContext();
    if (context === null) {
      return Promise.resolve();
    }
    return this.#reconcileForSession(reason, context);
  }

  #reconcileForSession(reason: ReconcileReason, context: SessionContext): Promise<void> {
    if (!this.#isCurrent(context)) {
      return Promise.resolve();
    }
    if (this.#reconcilePromise !== null) {
      this.#queuedReason = this.#mergeQueuedReason(this.#queuedReason, reason);
      return this.#reconcilePromise;
    }

    const reconciliation = this.#runReconciliations(reason, context).finally(() => {
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

    const context = this.#sessionContext();
    if (context === null) {
      return;
    }
    const signal = context.controller.signal;
    if (!this.#isCurrent(context)) {
      return;
    }
    this.loadingMore = true;
    this.loadMoreError = null;

    try {
      const page = await request<NotificationPage>(
        `/notifications?size=20&cursor=${encodeURIComponent(cursor)}`,
        { authenticated: true, signal },
      );
      if (!this.#isCurrent(context)) {
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
      if (this.#isCurrent(context)) {
        this.loadMoreError = toMessage(cause);
      }
    } finally {
      if (this.#isCurrent(context)) {
        this.loadingMore = false;
      }
    }
  }

  async markRead(id: string): Promise<void> {
    const notification = this.items.find((item) => item.id === id);
    if (!notification || notification.read_at !== null) {
      return;
    }

    const context = this.#sessionContext();
    if (context === null) {
      return;
    }
    return this.#enqueueMutation(() => this.#markRead(id, context));
  }

  async markAllRead(): Promise<void> {
    if (this.unreadCount === 0) {
      return;
    }

    const context = this.#sessionContext();
    if (context === null) {
      return;
    }
    const ids = new Set(this.items.filter((item) => item.read_at === null).map((item) => item.id));
    return this.#enqueueMutation(() => this.#markAllRead(ids, context));
  }

  #enqueueMutation(operation: () => Promise<void>): Promise<void> {
    const result = this.#mutationQueue.then(operation);
    this.#mutationQueue = result.catch(() => undefined);
    return result;
  }

  async #markRead(id: string, context: SessionContext): Promise<void> {
    if (!this.#isCurrent(context)) {
      return;
    }
    const signal = context.controller.signal;
    const notification = this.items.find((item) => item.id === id);
    if (!notification || notification.read_at !== null) {
      return;
    }
    const previousReadAt = notification.read_at;
    const previousCount = this.unreadCount;
    const readAt = new Date().toISOString();
    this.items = this.items.map((item) => (item.id === id ? { ...item, read_at: readAt } : item));
    this.unreadCount = Math.max(0, this.unreadCount - 1);
    this.error = null;

    try {
      const result = await request<NotificationReadResponse>(`/notifications/${id}/read`, {
        method: "PUT",
        authenticated: true,
        signal,
      });
      if (this.#isCurrent(context)) {
        this.items = this.items.map((item) =>
          item.id === id ? { ...item, read_at: readAt } : item,
        );
        this.unreadCount = result.unread_count;
      }
    } catch (cause) {
      if (this.#isCurrent(context)) {
        this.items = this.items.map((item) =>
          item.id === id ? { ...item, read_at: previousReadAt } : item,
        );
        this.unreadCount = previousCount;
        const message = toMessage(cause);
        await this.#reconcileForSession("recovery", context);
        if (this.#isCurrent(context)) {
          this.error = message;
        }
      }
      throw cause;
    }
  }

  async #markAllRead(ids: Set<string>, context: SessionContext): Promise<void> {
    if (!this.#isCurrent(context)) {
      return;
    }
    const signal = context.controller.signal;
    const previousReadAt = new Map(
      this.items.filter((item) => ids.has(item.id)).map((item) => [item.id, item.read_at]),
    );
    const previousCount = this.unreadCount;
    const readAt = new Date().toISOString();
    this.items = this.items.map((item) => (ids.has(item.id) ? { ...item, read_at: readAt } : item));
    this.unreadCount = Math.max(0, this.unreadCount - previousReadAt.size);
    this.error = null;

    try {
      const result = await request<NotificationReadResponse>("/notifications/read-all", {
        method: "PUT",
        authenticated: true,
        signal,
      });
      if (this.#isCurrent(context)) {
        this.items = this.items.map((item) =>
          ids.has(item.id) ? { ...item, read_at: readAt } : item,
        );
        this.unreadCount = result.unread_count;
      }
    } catch (cause) {
      if (this.#isCurrent(context)) {
        this.items = this.items.map((item) =>
          previousReadAt.has(item.id)
            ? { ...item, read_at: previousReadAt.get(item.id) ?? null }
            : item,
        );
        this.unreadCount = previousCount;
        const message = toMessage(cause);
        await this.#reconcileForSession("recovery", context);
        if (this.#isCurrent(context)) {
          this.error = message;
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

  async #runReconciliations(firstReason: ReconcileReason, context: SessionContext): Promise<void> {
    let reason: ReconcileReason | null = firstReason;
    while (reason !== null && this.#isCurrent(context)) {
      await this.#reconcileOnce(reason, context);
      if (!this.#isCurrent(context)) {
        return;
      }
      reason = this.#queuedReason;
      this.#queuedReason = null;
    }
  }

  async #reconcileOnce(reason: ReconcileReason, context: SessionContext): Promise<void> {
    if (!this.#isCurrent(context)) {
      return;
    }
    const signal = context.controller.signal;
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
      if (!this.#isCurrent(context)) {
        return;
      }

      const establishingBaseline = !this.#hasBaseline;
      const newlyDiscovered = establishingBaseline
        ? []
        : page.notifications.filter((notification) => !this.#knownIds.has(notification.id));
      this.items = [
        ...page.notifications,
        ...this.items.filter(
          (notification) => !page.notifications.some((current) => current.id === notification.id),
        ),
      ];
      this.unreadCount = page.unread_count;
      this.nextCursor = page.next_cursor;
      this.#hasBaseline = true;

      if (reason !== "initial" && newlyDiscovered.length > 0) {
        const versions = { ...this.#activityVersions };
        for (const notification of newlyDiscovered) {
          versions[notification.account_id] = (versions[notification.account_id] ?? 0) + 1;
        }
        this.#activityVersions = versions;
      }

      for (const notification of page.notifications) {
        this.#knownIds.add(notification.id);
        if (!establishingBaseline && reason === "live" && newlyDiscovered.includes(notification)) {
          this.#pendingLive.set(notification.id, notification);
        }
      }

      let accountsApplied = false;
      try {
        accountsApplied = await accounts.load(signal);
      } catch {
        accountsApplied = false;
      }
      if (!this.#isCurrent(context)) {
        return;
      }
      if (accountsApplied) {
        this.#publishPendingToasts(context);
      }
    } catch (cause) {
      if (this.#isCurrent(context)) {
        this.error = toMessage(cause);
      }
    } finally {
      if (this.#isCurrent(context)) {
        this.loading = false;
        this.refreshing = false;
      }
    }
  }

  #publishPendingToasts(context: SessionContext): void {
    if (!this.#isCurrent(context) || this.#pendingLive.size === 0) {
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
        if (this.#isCurrent(context)) {
          this.dismissToast(toast.id);
        }
      }, 5_000);
      this.#toastTimers.set(toast.id, timer);
    }
  }

  #queueReconcile(reason: ReconcileReason, context: SessionContext): void {
    void this.#reconcileForSession(reason, context);
  }

  #mergeQueuedReason(current: ReconcileReason | null, next: ReconcileReason): ReconcileReason {
    return current === "live" || next === "live" ? "live" : next;
  }

  async #streamLoop(context: SessionContext): Promise<void> {
    const signal = context.controller.signal;
    let reconnectIndex = 0;
    while (this.#isCurrent(context)) {
      try {
        const response = await requestResponse("/notifications/stream", {
          authenticated: true,
          signal,
        });
        if (!this.#isCurrent(context)) {
          return;
        }
        reconnectIndex = 0;
        await this.#reconcileForSession("connected", context);
        await consumeEventStream(response, () => this.#queueReconcile("live", context), signal);
      } catch {
        if (!this.#isCurrent(context)) {
          return;
        }
      }

      if (!this.#isCurrent(context)) {
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

  #sessionContext(): SessionContext | null {
    if (this.#controller === null && this.#sessionAuthGeneration === null) {
      this.#controller = new AbortController();
      this.#sessionAuthGeneration = auth.generation;
    }
    if (
      this.#controller === null ||
      this.#controller.signal.aborted ||
      this.#sessionAuthGeneration !== auth.generation
    ) {
      return null;
    }
    return {
      generation: this.#generation,
      authGeneration: this.#sessionAuthGeneration,
      controller: this.#controller,
    };
  }

  #isCurrent(context: SessionContext): boolean {
    return (
      this.#generation === context.generation &&
      this.#sessionAuthGeneration === context.authGeneration &&
      auth.generation === context.authGeneration &&
      this.#controller === context.controller &&
      !context.controller.signal.aborted
    );
  }
}

export const notifications = new NotificationsStore();
