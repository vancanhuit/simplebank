import { AxeBuilder } from "@axe-core/playwright";
import { expect, type Page } from "@playwright/test";

export interface Account {
  id: string;
  owner: string;
  balance: number;
  currency: "USD" | "EUR" | "VND";
  created_at: string;
}

export interface Notification {
  id: string;
  account_id: string;
  transfer_id: string;
  direction: "sent" | "received";
  amount: number;
  currency: Account["currency"];
  balance: number;
  read_at: string | null;
  created_at: string;
}

export interface NotificationPage {
  notifications: Notification[];
  unread_count: number;
  next_cursor: string | null;
}

interface User {
  username: string;
  full_name: string;
  email: string;
  is_email_verified: boolean;
  created_at: string;
}

export const user: User = {
  username: "alexandria",
  full_name: "Alexandria Montgomery-Worthington Alexandria Montgomery-Worthington",
  email: "alexandria@example.com",
  is_email_verified: true,
  created_at: "2026-01-01T00:00:00Z",
};

export const account: Account = {
  id: "11111111-2222-3333-4444-555566667777",
  owner: user.username,
  balance: 125_000,
  currency: "USD",
  created_at: "2026-01-15T10:00:00Z",
};

export const apiErrors = {
  rateLimited: { code: "rate_limited", error: "rate limit exceeded" },
  internal: { code: "internal_error", error: "internal server error" },
} as const;

const emptyNotifications: NotificationPage = {
  notifications: [],
  unread_count: 0,
  next_cursor: null,
};

export interface AuthenticatedApiMock {
  setAccounts(accounts: Account[]): void;
  setNotifications(page: NotificationPage): void;
  connectionGeneration(): Promise<number>;
  waitForConnectionAfter(previous: number): Promise<number>;
  emitNotification(id: string): Promise<void>;
  closeStream(): Promise<void>;
}

export async function mockAuthenticatedAPI(
  page: Page,
  initialAccounts: Account[] = [],
): Promise<AuthenticatedApiMock> {
  let accounts = initialAccounts;
  let notificationPage = emptyNotifications;

  await page.addInitScript(() => {
    const state = window as typeof window & {
      __notificationStreamController?: ReadableStreamDefaultController<Uint8Array>;
      __notificationStreamGeneration?: number;
    };
    const originalFetch = window.fetch.bind(window);
    const encoder = new TextEncoder();

    window.fetch = async (input, init) => {
      const url = new URL(input instanceof Request ? input.url : String(input), location.href);
      if (url.pathname !== "/api/v1/notifications/stream") {
        return originalFetch(input, init);
      }

      const stream = new ReadableStream<Uint8Array>({
        start(controller) {
          state.__notificationStreamController = controller;
          state.__notificationStreamGeneration = (state.__notificationStreamGeneration ?? 0) + 1;
          controller.enqueue(encoder.encode(": connected\n\n"));
        },
        cancel() {
          state.__notificationStreamController = undefined;
        },
      });
      return new Response(stream, {
        status: 200,
        headers: { "Content-Type": "text/event-stream", "Cache-Control": "no-store" },
      });
    };
  });

  await page.route("**/api/v1/tokens/renew", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        access_token: "test-token",
        access_token_expires_at: "2026-01-01T01:00:00Z",
        user,
      }),
    }),
  );
  await page.route("**/api/v1/notifications?*", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(notificationPage),
    }),
  );
  await page.route("**/api/v1/notifications/read-all", (route) => {
    notificationPage = {
      ...notificationPage,
      notifications: notificationPage.notifications.map((item) => ({
        ...item,
        read_at: item.read_at ?? "2026-08-23T12:00:00Z",
      })),
      unread_count: 0,
    };
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ unread_count: notificationPage.unread_count }),
    });
  });
  await page.route("**/api/v1/notifications/*/read", (route) => {
    const id = new URL(route.request().url()).pathname.split("/").at(-2);
    const wasUnread = notificationPage.notifications.some(
      (item) => item.id === id && item.read_at === null,
    );
    notificationPage = {
      ...notificationPage,
      notifications: notificationPage.notifications.map((item) =>
        item.id === id ? { ...item, read_at: "2026-08-23T12:00:00Z" } : item,
      ),
      unread_count: wasUnread
        ? Math.max(0, notificationPage.unread_count - 1)
        : notificationPage.unread_count,
    };
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ unread_count: notificationPage.unread_count }),
    });
  });
  // Register the specific account transfer route before broad account routes.
  await page.route(/\/api\/v1\/accounts\/[^/]+\/transfers\?/, (route) => {
    const id = new URL(route.request().url()).pathname.split("/").at(-2) ?? account.id;
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify([
        {
          id: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
          from_account_id: id,
          to_account_id: "99999999-8888-7777-6666-555544443333",
          amount: 25_00,
          idempotency_key: "11111111-aaaa-bbbb-cccc-222222222222",
          created_at: "2026-01-16T10:00:00Z",
        },
      ]),
    });
  });
  await page.route(/\/api\/v1\/accounts\/[^/?]+$/, (route) => {
    const id = new URL(route.request().url()).pathname.split("/").at(-1);
    const matched = accounts.find((item) => item.id === id) ?? account;
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(matched),
    });
  });
  await page.route("**/api/v1/accounts?*", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(accounts) }),
  );
  await page.route("**/api/v1/transfer-limits", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: "{}" }),
  );

  async function evaluateStream(operation: "emit" | "close", id?: string): Promise<void> {
    await page.evaluate(
      async ({ operation: requestedOperation, id: notificationId }) => {
        const state = window as typeof window & {
          __notificationStreamController?: ReadableStreamDefaultController<Uint8Array>;
        };
        const deadline = performance.now() + 2_000;
        while (!state.__notificationStreamController && performance.now() < deadline) {
          await new Promise((resolve) => setTimeout(resolve, 10));
        }
        const controller = state.__notificationStreamController;
        if (!controller) {
          throw new Error("notification stream was not opened");
        }
        if (requestedOperation === "close") {
          controller.close();
          state.__notificationStreamController = undefined;
          return;
        }
        controller.enqueue(
          new TextEncoder().encode(`event: notification\ndata: ${notificationId}\n\n`),
        );
      },
      { operation, id },
    );
  }

  return {
    setAccounts(nextAccounts) {
      accounts = nextAccounts;
    },
    setNotifications(nextPage) {
      notificationPage = nextPage;
    },
    connectionGeneration() {
      return page.evaluate(() => {
        const state = window as typeof window & { __notificationStreamGeneration?: number };
        return state.__notificationStreamGeneration ?? 0;
      });
    },
    async waitForConnectionAfter(previous) {
      await page.waitForFunction((oldGeneration) => {
        const state = window as typeof window & { __notificationStreamGeneration?: number };
        return (state.__notificationStreamGeneration ?? 0) > oldGeneration;
      }, previous);
      return page.evaluate(() => {
        const state = window as typeof window & { __notificationStreamGeneration?: number };
        return state.__notificationStreamGeneration ?? 0;
      });
    },
    emitNotification(id) {
      return evaluateStream("emit", id);
    },
    closeStream() {
      return evaluateStream("close");
    },
  };
}

export async function expectNoAccessibilityViolations(page: Page): Promise<void> {
  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
}
