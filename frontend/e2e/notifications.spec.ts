import { expect, test, type Page, type Route } from "@playwright/test";
import {
  account,
  mockAuthenticatedAPI,
  type Account,
  type Notification,
  type NotificationPage,
} from "./support/mock-api.js";

const sent: Notification = {
  id: "0198d94d-9380-7d00-8000-000000000001",
  account_id: account.id,
  transfer_id: "0198d94d-9380-7d00-8000-000000000011",
  direction: "sent",
  amount: 25_00,
  currency: "USD",
  balance: 122_500,
  read_at: null,
  created_at: "2026-08-23T11:00:00Z",
};

const received: Notification = {
  id: "0198d94d-9380-7d00-8000-000000000002",
  account_id: account.id,
  transfer_id: "0198d94d-9380-7d00-8000-000000000012",
  direction: "received",
  amount: 40_00,
  currency: "USD",
  balance: 129_000,
  read_at: null,
  created_at: "2026-08-23T11:01:00Z",
};

function withBalance(balance: number): Account {
  return { ...account, balance };
}

function notificationPage(
  notifications: Notification[],
  unreadCount = notifications.filter((item) => item.read_at === null).length,
  nextCursor: string | null = null,
): NotificationPage {
  return { notifications, unread_count: unreadCount, next_cursor: nextCursor };
}

async function openDashboard(page: Page, initialAccount = account) {
  const historyLoaded = page.waitForResponse((response) =>
    response.url().includes("/api/v1/notifications?size=20"),
  );
  const api = await mockAuthenticatedAPI(page, [initialAccount]);
  await page.goto("/");
  await historyLoaded;
  await expect(page.getByText("$1,250.00", { exact: true }).first()).toBeVisible();
  await expect(page.getByRole("button", { name: "Notifications, 0 unread" })).toBeVisible();
  return api;
}

for (const scenario of [
  { name: "sender", item: sent, balance: 122_500, amount: "−$25.00", label: "Sent" },
  {
    name: "recipient",
    item: received,
    balance: 129_000,
    amount: "+$40.00",
    label: "Received",
  },
] as const) {
  test(`${scenario.name} sees live balance, badge, toast, and mark-before-navigation`, async ({
    page,
  }) => {
    const api = await openDashboard(page);
    api.setAccounts([withBalance(scenario.balance)]);
    api.setNotifications(notificationPage([scenario.item]));

    const started = Date.now();
    await api.emitNotification(scenario.item.id);
    await expect(
      page.getByText(scenario.balance === 122_500 ? "$1,225.00" : "$1,290.00").first(),
    ).toBeVisible({ timeout: 900 });
    await expect(page.getByRole("button", { name: "Notifications, 1 unread" })).toBeVisible({
      timeout: 900,
    });
    expect(Date.now() - started).toBeLessThan(1_000);

    const toast = page
      .locator(".toast")
      .getByText(
        new RegExp(`${scenario.label}\\s*${scenario.amount.replace(/[+$]/g, "\\$&")}\\s*USD`),
      );
    await expect(toast).toHaveCount(1);

    await page.getByRole("button", { name: "Notifications, 1 unread" }).click();
    const item = page.getByRole("button", {
      name: new RegExp(`${scenario.label}, ${scenario.amount.replace(/[+$]/g, "\\$&")}.*unread`),
    });
    let releaseMark!: () => void;
    const markReleased = new Promise<void>((resolve) => {
      releaseMark = resolve;
    });
    await page.route(`**/api/v1/notifications/${scenario.item.id}/read`, async (route) => {
      await markReleased;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ unread_count: 0 }),
      });
    });
    const marked = page.waitForResponse(
      (response) =>
        response.url().endsWith(`/notifications/${scenario.item.id}/read`) && response.ok(),
    );
    const activation = item.click();
    await expect(page).toHaveURL("/");
    releaseMark();
    await activation;
    await marked;
    await expect(page).toHaveURL(`/accounts/${account.id}`);
    await expect(page.getByRole("button", { name: "Notifications, 0 unread" })).toBeVisible();
  });
}

test("a burst coalesces to one follow-up reconciliation and deduplicates toast ids", async ({
  page,
}) => {
  const api = await openDashboard(page);
  api.setAccounts([withBalance(received.balance)]);
  const finalPage = notificationPage([received, sent], 2);
  let historyRequests = 0;
  let releaseFirst!: () => void;
  const firstRequestHeld = new Promise<void>((resolve) => {
    releaseFirst = resolve;
  });
  const burstRoute = async (route: Route) => {
    historyRequests += 1;
    if (historyRequests === 1) {
      await firstRequestHeld;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(finalPage),
    });
  };
  await page.route("**/api/v1/notifications?*", burstRoute);

  await api.emitNotification(sent.id);
  await expect.poll(() => historyRequests).toBe(1);
  await api.emitNotification(received.id);
  await api.emitNotification(sent.id);
  releaseFirst();

  await expect(page.getByRole("button", { name: "Notifications, 2 unread" })).toBeVisible();
  await expect.poll(() => historyRequests).toBe(2);
  await expect(page.locator(".toast .alert")).toHaveCount(2);
  await expect(page.locator(".toast").getByText(/Sent\s*−\$25\.00\s*USD/)).toHaveCount(1);
  await expect(page.locator(".toast").getByText(/Received\s*\+\$40\.00\s*USD/)).toHaveCount(1);
  await page.unroute("**/api/v1/notifications?*", burstRoute);
});

test("reconnect reconciliation restores durable state without replaying a toast", async ({
  page,
}) => {
  const api = await openDashboard(page);
  await api.closeStream();
  api.setAccounts([withBalance(received.balance)]);
  api.setNotifications(notificationPage([received]));

  await expect(page.getByText("$1,290.00", { exact: true }).first()).toBeVisible({
    timeout: 2_000,
  });
  await expect(page.getByRole("button", { name: "Notifications, 1 unread" })).toBeVisible();
  await expect(page.locator(".toast .alert")).toHaveCount(0);

  await page.getByRole("button", { name: "Notifications, 1 unread" }).click();
  await expect(page.getByRole("button", { name: /Received.*unread/ })).toBeVisible();
});

test("history supports pagination, retained-data errors, mark-all, keyboard activation, and back", async ({
  page,
}) => {
  const firstPage = notificationPage([received], 2, "older");
  const older = { ...sent, read_at: "2026-08-23T12:00:00Z" };
  const secondPage = notificationPage([older], 2);
  const api = await mockAuthenticatedAPI(page, [account]);
  api.setNotifications(firstPage);
  let failOlder = true;
  const historyRoute = async (route: Route) => {
    const url = new URL(route.request().url());
    if (url.searchParams.get("cursor") === "older") {
      if (failOlder) {
        await route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ error: "History temporarily unavailable" }),
        });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(secondPage),
      });
      return;
    }
    await route.fallback();
  };
  await page.route("**/api/v1/notifications?*", historyRoute);
  await page.goto("/notifications");
  await expect(page.getByRole("heading", { name: "Notifications" })).toBeVisible();
  await expect(page.getByRole("button", { name: /Received.*unread/ })).toBeVisible();

  await page.getByRole("button", { name: "Load more" }).click();
  await expect(page.getByRole("alert")).toContainText("History temporarily unavailable");
  await expect(page.getByRole("button", { name: /Received.*unread/ })).toBeVisible();
  failOlder = false;
  await page.getByRole("button", { name: "Retry" }).click();
  await expect(page.getByRole("button", { name: /Sent/ })).toBeVisible();

  await page.getByRole("button", { name: "Mark all read" }).click();
  await expect(page.getByRole("button", { name: "Notifications, 0 unread" })).toBeVisible();

  const row = page.getByRole("button", { name: /Received/ });
  await row.focus();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(`/accounts/${account.id}`);
  await page.goBack();
  await expect(page).toHaveURL("/notifications");
  await expect(page.getByRole("heading", { name: "Notifications" })).toBeVisible();
});
