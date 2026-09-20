import { expect, test } from "@playwright/test";
import {
  account,
  user,
  mockAuthenticatedAPI,
  expectNoAccessibilityViolations,
  type Notification,
} from "./support/mock-api.js";

test("expired-link recovery accepts a public resend and a replacement link verifies without login", async ({
  page,
}) => {
  await page.route("**/api/v1/tokens/renew", (route) => route.fulfill({ status: 204 }));
  await page.route("**/api/v1/users/verify_email?*", (route) =>
    route.fulfill(
      route.request().url().includes("code=replacement")
        ? { json: { is_verified: true } }
        : { status: 400, json: { code: "invalid_verification_link" } },
    ),
  );
  await page.route("**/api/v1/users/verify_email/resend", (route) =>
    route.fulfill({ status: 202, json: { message: "accepted" } }),
  );
  await page.goto("/verify-email?id=record&code=expired");
  await expect(page.getByRole("alert")).toContainText("invalid or has expired");
  await expect(page).toHaveURL(/\/verify-email$/);
  await page.getByRole("textbox", { name: "Email" }).fill("alice@example.com");
  await page.getByRole("button", { name: "Send verification email" }).click();
  await expect(page.getByRole("status")).toContainText("Request accepted.");
  await expectNoAccessibilityViolations(page);
  await page.goto("/verify-email?id=record&code=replacement");
  await expect(page.getByRole("heading", { name: "Email verified" })).toBeVisible();
  await expect(page).toHaveURL(/\/verify-email$/);
  await expect(page.getByRole("link", { name: "Continue to sign in" }).first()).toBeVisible();
});

test.describe("German exact currency display", () => {
  test.use({ locale: "de-DE" });
  test("keeps all cents, grouping and currency placement at the safe boundary", async ({
    page,
  }) => {
    await mockAuthenticatedAPI(page, [
      { ...account, currency: "EUR", balance: Number.MAX_SAFE_INTEGER },
    ]);
    await page.goto("/transfer");
    await expect(page.getByText("Available: 90.071.992.547.409,91 €")).toBeVisible();
  });
});

const note: Notification = {
  id: "alice-note",
  account_id: account.id,
  transfer_id: "alice-transfer",
  direction: "sent",
  amount: 123,
  currency: "USD",
  balance: account.balance,
  read_at: null,
  created_at: "2026-01-01T00:00:00Z",
};

test("shared httpOnly cookie replacement resets the first tab and resumes Bob notifications", async ({
  page,
  context,
}) => {
  const api = await mockAuthenticatedAPI(page, [account]);
  const bob = { ...user, username: "bob", full_name: "Bob", email: "bob@example.com" };
  const bobAccount = { ...account, id: "bob-account", owner: "bob", balance: 20000 };
  await context.addCookies([
    { name: "refresh", value: "alice", url: "http://127.0.0.1:5173", httpOnly: true },
  ]);
  await page.route("**/api/v1/tokens/renew", async (route) => {
    const cookie = await route.request().headerValue("cookie");
    const principal = cookie?.includes("refresh=bob") ? bob : user;
    await route.fulfill({
      json: {
        user: principal,
        access_token: principal.username,
        access_token_expires_at: "2026-12-01T00:00:00Z",
      },
    });
  });
  api.setNotifications({ notifications: [note], unread_count: 1, next_cursor: null });
  await page.goto("/transfer");
  await expect(page.getByRole("combobox", { name: "From account" })).toHaveValue(account.id);
  await page.getByRole("textbox", { name: "Amount (USD)" }).fill("12.34");
  await page.getByRole("textbox", { name: "Recipient account id" }).fill("alice-recipient");
  const second = await context.newPage();
  await mockAuthenticatedAPI(second, [bobAccount]);
  await second.route("**/api/v1/tokens/renew", (route) => route.fulfill({ status: 204 }));
  await second.route("**/api/v1/users/login", (route) =>
    route.fulfill({
      headers: { "Set-Cookie": "refresh=bob; Path=/; HttpOnly; SameSite=Lax" },
      json: {
        user: bob,
        access_token: "bob",
        access_token_expires_at: "2026-12-01T00:00:00Z",
        session_id: "bob-session",
      },
    }),
  );
  await second.goto("/login");
  await second.getByRole("textbox", { name: "Username" }).fill("bob");
  await second.getByLabel("Password", { exact: true }).fill("correct password");
  await second.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect
    .poll(async () => (await context.cookies()).find((cookie) => cookie.name === "refresh")?.value)
    .toBe("bob");
  await second.close();
  let expired = false;
  await page.route("**/api/v1/notifications?*", async (route) => {
    if (!expired) {
      expired = true;
      await route.fulfill({ status: 401, json: { code: "token_expired" } });
      return;
    }
    await route.fallback();
  });
  api.setAccounts([bobAccount]);
  api.setNotifications({ notifications: [], unread_count: 0, next_cursor: null });
  await page.bringToFront();
  await page.evaluate(() => {
    const violations: string[] = [];
    const observer = new MutationObserver(() => {
      if (
        document.body.textContent?.includes("Bob") &&
        (document.querySelector('#notification-preview button[aria-label*="1.23"]') !== null ||
          document.querySelector(".toast")?.textContent?.includes("1.23") ||
          document.querySelector<HTMLInputElement>(
            'input[placeholder="00000000-0000-0000-0000-000000000000"]',
          )?.value === "alice-recipient" ||
          document.querySelector<HTMLSelectElement>("#from")?.value ===
            "11111111-2222-3333-4444-555566667777")
      )
        violations.push("mixed principal DOM");
    });
    observer.observe(document.body, {
      subtree: true,
      childList: true,
      characterData: true,
      attributes: true,
    });
    Object.assign(window, { auditViolations: violations });
  });
  await api.emitNotification("expiry");
  await expect(page.getByRole("combobox", { name: "From account" })).toHaveValue(bobAccount.id);
  await expect(page.getByText("Bob", { exact: true })).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Recipient account id" })).toHaveValue("");
  await expect(page.getByRole("textbox", { name: "Amount (USD)" })).toHaveValue("");
  await expect(page.getByRole("button", { name: "Notifications, 0 unread" })).toBeVisible();
  expect(await page.evaluate((): unknown => Reflect.get(window, "auditViolations"))).toEqual([]);
  api.setNotifications({
    notifications: [{ ...note, id: "bob-note", account_id: bobAccount.id, amount: 456 }],
    unread_count: 1,
    next_cursor: null,
  });
  await api.emitNotification("bob-note");
  await expect(page.getByRole("button", { name: "Notifications, 1 unread" })).toBeVisible();
  await page.getByRole("button", { name: "Notifications, 1 unread" }).click();
  await expect(page.getByRole("button", { name: /Sent.*4.56/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /Sent.*1.23/ })).toHaveCount(0);
});

test("money text is exact in the browser and keyboard-cleared deposits open at zero", async ({
  page,
}) => {
  const api = await mockAuthenticatedAPI(page, []);
  await page.route("**/api/v1/account-opening-limits", (route) =>
    route.fulfill({ json: { USD: Number.MAX_SAFE_INTEGER, EUR: Number.MAX_SAFE_INTEGER } }),
  );
  const balances: number[] = [];
  await page.route("**/api/v1/accounts", async (route) => {
    const body = route.request().postDataJSON() as { balance: number };
    balances.push(body.balance);
    if (body.balance === 0) {
      const created = { ...account, balance: 0 };
      api.setAccounts([created]);
      await route.fulfill({ status: 200, json: created });
    } else await route.fulfill({ status: 503, json: { code: "internal_error" } });
  });
  await page.goto("/accounts/new");
  const deposit = page.getByRole("textbox", { name: "Opening deposit (USD)" });
  await expect(deposit).toBeEnabled();
  await expect(deposit).toHaveAccessibleDescription(/90,071,992,547,409.91/);
  await deposit.fill("90071992547409.91");
  await page.getByRole("button", { name: "Create account" }).click();
  await expect.poll(() => balances).toEqual([Number.MAX_SAFE_INTEGER]);
  await deposit.focus();
  await deposit.press("ControlOrMeta+A");
  await deposit.press("Backspace");
  await page.getByRole("button", { name: "Create account" }).click();
  await expect.poll(() => balances).toEqual([Number.MAX_SAFE_INTEGER, 0]);
  await expect(page).toHaveURL(/\/$/);
  api.setAccounts([{ ...account, balance: Number.MAX_SAFE_INTEGER }]);
  await page.goto("/transfer");
  await expect(page.getByText("Available: $90,071,992,547,409.91")).toBeVisible();
  await page.getByRole("textbox", { name: "Recipient account id" }).fill("recipient");
  const amount = page.getByRole("textbox", { name: "Amount (USD)" });
  await amount.fill("12.34");
  await amount.press("ControlOrMeta+A");
  await amount.press("Backspace");
  await page.getByRole("button", { name: "Send transfer" }).click();
  await expect(amount).toHaveAttribute("aria-invalid", "true");
  await expect(amount).toBeFocused();
  let submittedAmount: number | undefined;
  await page.route("**/api/v1/transfers", (route) => {
    const body = route.request().postDataJSON() as { amount: number };
    submittedAmount = body.amount;
    return route.fulfill({ status: 422, json: { code: "insufficient_balance" } });
  });
  await amount.fill("90071992547409.91");
  await page.getByRole("button", { name: "Send transfer" }).click();
  await expect.poll(() => submittedAmount).toBe(Number.MAX_SAFE_INTEGER);
  await expect(page.getByRole("alert")).toContainText("don't have enough money");
});

test("notification-triggered account refresh retains focused controls through success, failure and retry", async ({
  page,
}) => {
  const api = await mockAuthenticatedAPI(page, [account]);
  await page.goto("/transfer");
  const amount = page.getByRole("textbox", { name: "Amount (USD)" });
  await expect(page.getByRole("combobox", { name: "From account" })).toHaveValue(account.id);
  await amount.fill("12.340");
  await page.getByRole("textbox", { name: "Recipient account id" }).fill("recipient");
  await amount.focus();
  let release!: () => void;
  let reached!: () => void;
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  const started = new Promise<void>((resolve) => {
    reached = resolve;
  });
  await page.route("**/api/v1/accounts?*", async (route) => {
    reached();
    await held;
    await route.fulfill({ json: [{ ...account, balance: 99900 }] });
  });
  await api.emitNotification("refresh");
  await started;
  await expect(amount).toBeFocused();
  await expect(amount).toHaveValue("12.340");
  release();
  await expect(page.getByText("Available: $999.00")).toBeVisible();
  await expect(amount).toBeFocused();
  await page.unroute("**/api/v1/accounts?*");
  await page.route("**/api/v1/accounts?*", (route) =>
    route.fulfill({ status: 503, json: { code: "internal_error" } }),
  );
  await api.emitNotification("failure");
  await expect(page.getByRole("alert")).toContainText("temporarily unavailable");
  await expect(amount).toBeFocused();
  await page.unroute("**/api/v1/accounts?*");
  await page.route("**/api/v1/accounts?*", (route) =>
    route.fulfill({ json: [{ ...account, id: "replacement" }] }),
  );
  await page.getByRole("button", { name: "Retry", exact: true }).click();
  await expect(page.getByRole("combobox", { name: "From account" })).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  await expect(page.getByRole("textbox", { name: "Amount", exact: true })).toHaveValue("12.340");
  await expect(page.getByRole("textbox", { name: "Recipient account id" })).toHaveValue(
    "recipient",
  );
});

test("public unverified-login recovery validates email, respects cooldown and is keyboard accessible", async ({
  page,
}) => {
  await page.route("**/api/v1/tokens/renew", (route) => route.fulfill({ status: 204 }));
  await page.route("**/api/v1/users/login", (route) =>
    route.fulfill({ status: 403, json: { code: "email_verification_required" } }),
  );
  let requests = 0;
  await page.route("**/api/v1/users/verify_email/resend", async (route) => {
    requests += 1;
    expect(route.request().postDataJSON()).toEqual({ email: "alice@example.com" });
    await route.fulfill(
      requests === 1
        ? { status: 429, headers: { "Retry-After": "1" }, json: { code: "rate_limited" } }
        : { status: 202, json: { message: "accepted" } },
    );
  });
  await page.goto("/login");
  await page.getByRole("textbox", { name: "Username" }).fill("alice");
  await page.getByLabel("Password", { exact: true }).fill("private password");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await page.getByRole("link", { name: "Request a new verification email" }).click();
  await expect(page).toHaveURL(/\/verify-email$/);
  const email = page.getByRole("textbox", { name: "Email" });
  await expect(email).toHaveValue("");
  await page.getByRole("button", { name: "Send verification email" }).click();
  await expect(email).toBeFocused();
  await expect(email).toHaveAttribute("aria-invalid", "true");
  await email.fill("alice@example.com");
  await email.press("Enter");
  const submit = page.getByRole("button", { name: "Send verification email" });
  await expect(submit).toBeDisabled();
  await expect(submit).toBeEnabled({ timeout: 3000 });
  await submit.focus();
  await submit.press("Enter");
  await expect(page.getByRole("status")).toContainText("Request accepted.");
  await expectNoAccessibilityViolations(page);
  expect(requests).toBe(2);
});

test("cross-tab read reconciliation refreshes the loaded history and reconnects without old toasts", async ({
  page,
}) => {
  const api = await mockAuthenticatedAPI(page, [account]);
  let read = false;
  const older = { ...note, id: "older-note", amount: 456 };
  await page.route("**/api/v1/notifications?*", (route) => {
    const row = new URL(route.request().url()).searchParams.has("cursor") ? older : note;
    return route.fulfill({
      json: {
        notifications: [{ ...row, read_at: read ? "2026-01-02T00:00:00Z" : null }],
        unread_count: read ? 0 : 2,
        next_cursor: row === note ? "older" : null,
      },
    });
  });
  await page.goto("/notifications");
  await page.getByRole("button", { name: "Load more" }).click();
  await expect(page.getByRole("main").getByRole("button", { name: /Sent.*unread/ })).toHaveCount(2);
  read = true;
  const generation = await api.connectionGeneration();
  await api.closeStream();
  await api.waitForConnectionAfter(generation);
  await expect(page.getByRole("main").getByRole("button", { name: /Sent.*unread/ })).toHaveCount(0);
  await expect(page.getByRole("main").getByRole("button", { name: /Sent/ })).toHaveCount(2);
  await expect(page.getByRole("button", { name: "Notifications, 0 unread" })).toBeVisible();
  await expect(page.locator(".toast .alert")).toHaveCount(0);
});
