import { expect, test } from "@playwright/test";
import {
  account,
  apiErrors,
  expectNoAccessibilityViolations,
  mockAuthenticatedAPI,
  type Notification,
  user,
} from "./support/mock-api.js";

const viewports = [
  { width: 320, height: 800 },
  { width: 768, height: 900 },
  { width: 1024, height: 900 },
  { width: 1440, height: 1000 },
];

const notification: Notification = {
  id: "0198d94d-9380-7d00-8000-000000000101",
  account_id: account.id,
  transfer_id: "0198d94d-9380-7d00-8000-000000000111",
  direction: "received",
  amount: 40_00,
  currency: "USD",
  balance: 129_000,
  read_at: null,
  created_at: "2026-08-23T11:01:00Z",
};

async function expectNoHorizontalOverflow(page: import("@playwright/test").Page): Promise<void> {
  const dimensions = await page.evaluate(() => ({
    viewport: document.documentElement.clientWidth,
    content: document.documentElement.scrollWidth,
  }));
  expect(dimensions.content).toBeLessThanOrEqual(dimensions.viewport);
}

async function expectMinimumInteractiveTargets(
  page: import("@playwright/test").Page,
  selector: string,
): Promise<void> {
  await expect
    .poll(() =>
      page.locator(selector).evaluateAll((elements) =>
        elements
          .filter((element) => {
            const style = getComputedStyle(element);
            return style.display !== "none" && style.visibility !== "hidden";
          })
          .map((element) => {
            const rectangle = element.getBoundingClientRect();
            return {
              label: element.getAttribute("aria-label") ?? element.textContent,
              width: rectangle.width,
              height: rectangle.height,
            };
          })
          // Chromium can report a 44px CSS target a few thousandths under 44
          // due to subpixel layout. Keep the tolerance far below a pixel.
          .filter(({ width, height }) => width < 43.9 || height < 43.9),
      ),
    )
    .toEqual([]);
}

test("theme selection is accessible, persisted, and valid after reload", async ({ page }) => {
  await mockAuthenticatedAPI(page);
  await page.emulateMedia({ colorScheme: "light" });
  await page.goto("/");

  await expect(page.locator("html")).toHaveAttribute("data-theme", "simplebank-light");
  await page.getByRole("button", { name: "Switch to dark theme" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "simplebank-dark");
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "simplebank-dark");
  await expect(page.getByRole("button", { name: "Switch to light theme" })).toBeVisible();
  await expectNoAccessibilityViolations(page);
});

test("system dark preference initializes the dark theme without a saved value", async ({
  page,
}) => {
  await mockAuthenticatedAPI(page);
  await page.emulateMedia({ colorScheme: "dark" });
  await page.goto("/");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "simplebank-dark");
});

for (const theme of ["simplebank-light", "simplebank-dark"] as const) {
  test(`login remains accessible in ${theme}`, async ({ page }) => {
    await page.addInitScript(({ key, value }) => localStorage.setItem(key, value), {
      key: "simplebank-theme",
      value: theme,
    });
    await page.route("**/api/v1/tokens/renew", (route) => route.fulfill({ status: 204 }));
    await page.goto("/login");
    await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
    await expect(page.getByRole("heading", { name: "Welcome back" })).toBeVisible();
    await expectNoAccessibilityViolations(page);
  });
}

test("empty registration focuses the first invalid field and remains accessible at 320", async ({
  page,
}) => {
  await page.route("**/api/v1/tokens/renew", (route) => route.fulfill({ status: 204 }));
  await page.setViewportSize({ width: 320, height: 800 });
  await page.goto("/register");

  await page.getByRole("button", { name: "Create account" }).click();

  const fullName = page.getByRole("textbox", { name: "Full name" });
  await expect(fullName).toBeFocused();
  await expect(fullName).toHaveAttribute("aria-invalid", "true");
  await expectNoAccessibilityViolations(page);
});

test("dashboard reflows and remains accessible at supported viewports", async ({ page }) => {
  await mockAuthenticatedAPI(page);

  for (const viewport of viewports) {
    await page.setViewportSize(viewport);
    await page.goto("/");
    await expect(page.getByRole("heading", { name: /Good to see you/ })).toBeVisible();
    await expect(page.getByRole("heading", { name: "No accounts yet" })).toBeVisible();
    await expect(page.getByRole("link", { name: "SimpleBank" })).toHaveCSS("min-height", "44px");

    const dimensions = await page.evaluate(() => ({
      viewport: document.documentElement.clientWidth,
      content: document.documentElement.scrollWidth,
    }));
    expect(dimensions.content).toBeLessThanOrEqual(dimensions.viewport);

    if (viewport.width < 640) {
      const menu = page.getByRole("button", { name: "Open navigation" });
      await expect(menu).toHaveCSS("min-height", "44px");

      if (viewport.width === 320) {
        const controls = await Promise.all(
          [
            menu,
            page.getByRole("link", { name: "SimpleBank" }),
            page.getByRole("button", { name: /Notifications, \d+ unread/ }),
            page.getByRole("button", { name: /switch to (dark|light) theme/i }),
            page.getByRole("button", { name: "Sign out" }),
          ].map((control) => control.boundingBox()),
        );
        expect(controls.every((box) => box !== null)).toBe(true);
        for (let index = 1; index < controls.length; index += 1) {
          expect(controls[index - 1]!.x + controls[index - 1]!.width).toBeLessThanOrEqual(
            controls[index]!.x,
          );
        }
      }

      await menu.focus();
      await page.keyboard.press("Enter");
      await expect(page.getByRole("navigation", { name: "Mobile primary" })).toBeVisible();
      await expect(page.getByRole("link", { name: "Transfer", exact: true }).last()).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(page.getByRole("navigation", { name: "Mobile primary" })).toBeHidden();
      await expect(menu).toBeFocused();

      await menu.click();
      await page.getByRole("link", { name: "Overview", exact: true }).last().click();
      await expect(page.getByRole("navigation", { name: "Mobile primary" })).toBeHidden();
      await expect(page.getByRole("button", { name: "Open navigation" })).toBeFocused();
    } else {
      await expect(page.getByRole("navigation", { name: "Primary", exact: true })).toBeVisible();
    }

    if (viewport.width === 320 || viewport.width === 1440) {
      for (const theme of ["simplebank-light", "simplebank-dark"] as const) {
        await page.evaluate(({ key, value }) => localStorage.setItem(key, value), {
          key: "simplebank-theme",
          value: theme,
        });
        await page.reload();
        await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
        await expect(page.getByRole("heading", { name: /Good to see you/ })).toBeVisible();
        await expectNoAccessibilityViolations(page);
      }
    }
  }
});

test("notification popover and history remain accessible and responsive in both themes", async ({
  page,
}) => {
  const api = await mockAuthenticatedAPI(page, [account]);
  api.setNotifications({ notifications: [notification], unread_count: 1, next_cursor: null });
  await page.goto("/");

  for (const viewport of [
    { width: 320, height: 800 },
    { width: 1440, height: 1000 },
  ]) {
    await page.setViewportSize(viewport);
    for (const theme of ["simplebank-light", "simplebank-dark"] as const) {
      await page.evaluate(({ key, value }) => localStorage.setItem(key, value), {
        key: "simplebank-theme",
        value: theme,
      });
      await page.reload();
      await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
      const bell = page.getByRole("button", { name: "Notifications, 1 unread" });
      await bell.focus();
      await page.keyboard.press("Enter");
      const popover = page.getByRole("region", { name: "Recent notifications" });
      await expect(popover).toBeVisible();
      await expectNoHorizontalOverflow(page);
      await expectMinimumInteractiveTargets(
        page,
        "#notification-preview button, #notification-preview a",
      );
      await page.waitForFunction(() =>
        document.getAnimations().every((animation) => animation.playState !== "running"),
      );
      await expectNoAccessibilityViolations(page);

      await page.keyboard.press("Escape");
      await expect(popover).toBeHidden();
      await expect(bell).toBeFocused();

      await bell.click();
      await page.getByRole("link", { name: "View all notifications" }).click();
      await expect(page).toHaveURL("/notifications");
      await expect(popover).toBeHidden();
      await page.waitForFunction(() =>
        document.getAnimations().every((animation) => animation.playState !== "running"),
      );
      await expectNoHorizontalOverflow(page);
      await expectMinimumInteractiveTargets(page, "main button, main a");
      await expectNoAccessibilityViolations(page);
    }
  }
});

test("live notification toast does not steal keyboard focus", async ({ page }) => {
  const api = await mockAuthenticatedAPI(page, [account]);
  await page.goto("/");
  await expect(page.getByRole("button", { name: "Notifications, 0 unread" })).toBeVisible();
  const themeToggle = page.getByRole("button", { name: /Switch to .* theme/ });
  await themeToggle.focus();

  api.setAccounts([{ ...account, balance: notification.balance }]);
  api.setNotifications({ notifications: [notification], unread_count: 1, next_cursor: null });
  await api.emitNotification(notification.id);

  await expect(page.locator(".toast").getByText(/Received\s*\+\$40\.00\s*USD/)).toBeVisible();
  await expect(themeToggle).toBeFocused();
});

test("long identity stays within the desktop header", async ({ page }) => {
  await mockAuthenticatedAPI(page);
  await page.setViewportSize({ width: 768, height: 900 });
  await page.goto("/");

  const identity = page.getByText(user.full_name);
  const header = page.getByRole("banner");
  await expect(identity).toHaveCSS("text-overflow", "ellipsis");

  const [identityBox, headerBox, signOutBox] = await Promise.all([
    identity.boundingBox(),
    header.boundingBox(),
    page.getByRole("button", { name: "Sign out" }).boundingBox(),
  ]);
  expect(identityBox).not.toBeNull();
  expect(headerBox).not.toBeNull();
  expect(signOutBox).not.toBeNull();
  expect(identityBox!.y).toBeGreaterThanOrEqual(headerBox!.y);
  expect(identityBox!.y + identityBox!.height).toBeLessThanOrEqual(
    headerBox!.y + headerBox!.height,
  );
  expect(signOutBox!.height).toBeGreaterThanOrEqual(44);
});

test("validation focuses and announces the first invalid field", async ({ page }) => {
  await mockAuthenticatedAPI(page, [account]);
  await page.setViewportSize({ width: 320, height: 800 });
  await page.goto("/transfer");
  await expect(page.getByRole("combobox", { name: "From account" })).toHaveValue(account.id);

  await page.getByRole("button", { name: "Send transfer" }).click();

  const recipient = page.getByRole("textbox", { name: "Recipient account id" });
  await expect(recipient).toBeFocused();
  await expect(recipient).toHaveAttribute("aria-invalid", "true");
  await expect(page.getByRole("alert")).toHaveText("Enter the recipient account id.");
  await expectNoAccessibilityViolations(page);
});

test("transfer account-load failure exposes an accessible retry state at 320", async ({ page }) => {
  await mockAuthenticatedAPI(page);
  let accountsUnavailable = true;
  await page.route("**/api/v1/accounts?*", (route) => {
    if (!accountsUnavailable) {
      return route.fallback();
    }
    return route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify(apiErrors.internal),
    });
  });
  await page.setViewportSize({ width: 320, height: 800 });
  await page.goto("/transfer");

  const alert = page.getByRole("alert");
  const retry = page.getByRole("button", { name: "Retry" });
  await expect(alert).toContainText(
    "We couldn't load your accounts. SimpleBank is temporarily unavailable. Please try again.",
  );
  await expect(retry).toBeVisible();
  await expectNoAccessibilityViolations(page);

  accountsUnavailable = false;
  await retry.click();
  await expect(alert).toBeHidden();
  await expect(page.getByRole("heading", { name: "Send money" })).toBeVisible();
  await expectNoAccessibilityViolations(page);
});

test("startup session restore recovers on the requested transfer page", async ({ page }) => {
  await mockAuthenticatedAPI(page, [account]);
  let renewalUnavailable = true;
  await page.route("**/api/v1/tokens/renew", (route) => {
    if (!renewalUnavailable) {
      return route.fallback();
    }
    return route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify(apiErrors.internal),
    });
  });

  await page.goto("/transfer");

  await expect(
    page.getByRole("heading", { name: "We couldn't restore your session." }),
  ).toBeVisible();
  await expect(page.getByRole("heading", { name: "Welcome back" })).toBeHidden();
  const retry = page.getByRole("button", { name: "Retry" });
  await expect(retry).toBeVisible();

  renewalUnavailable = false;
  await retry.click();

  await expect(page).toHaveURL("/transfer");
  await expect(page.getByRole("heading", { name: "Send money" })).toBeVisible();
});

test("browser history restores dashboard title and focus", async ({ page }) => {
  await mockAuthenticatedAPI(page, [account]);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/");

  await expect(page).toHaveTitle(/Dashboard/);
  await page.getByRole("link", { name: "Transfer", exact: true }).click();
  await expect(page).toHaveTitle(/Send money/);

  await page.goBack();
  await expect(page).toHaveTitle(/Dashboard/);
  await expect(page.locator("main")).toBeFocused();
});

test("account activity displays sent transfers and remains accessible at 320 and 1440", async ({
  page,
}) => {
  await mockAuthenticatedAPI(page, [account]);

  for (const viewport of [
    { width: 320, height: 800 },
    { width: 1440, height: 1000 },
  ]) {
    await page.setViewportSize(viewport);
    await page.goto(`/accounts/${account.id}`);

    // Wait for loaded activity state before checking overflow/Axe
    await expect(page.getByRole("heading", { name: "Activity" })).toBeVisible();
    await expect(page.getByText("Sent")).toBeVisible();
    await expect(page.getByText("−$25.00")).toBeVisible();

    const dimensions = await page.evaluate(() => ({
      viewport: document.documentElement.clientWidth,
      content: document.documentElement.scrollWidth,
    }));
    expect(dimensions.content).toBeLessThanOrEqual(dimensions.viewport);

    await expectNoAccessibilityViolations(page);
  }
});

test("local IBM Plex Sans Variable font loads successfully", async ({ page }) => {
  await mockAuthenticatedAPI(page);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/");

  await expect(page.getByRole("heading", { name: /Good to see you/ })).toBeVisible();

  // Wait for document.fonts.ready to ensure fonts are loaded
  await page.evaluate(() => document.fonts.ready);

  // Prove actual local .woff2 font loaded from same origin (not external CDN)
  const localFontLoaded = await page.evaluate(() => {
    const entries = performance.getEntriesByType("resource");
    const pageUrl = new URL(window.location.href);
    return entries.some((entry) => {
      const resourceUrl = new URL(entry.name);
      return (
        entry.name.includes(".woff2") &&
        entry.name.includes("plex") &&
        resourceUrl.origin === pageUrl.origin
      );
    });
  });
  expect(localFontLoaded).toBe(true);

  // Verify IBM Plex Sans Variable is available
  const fontAvailable = await page.evaluate(() =>
    document.fonts.check('16px "IBM Plex Sans Variable"'),
  );
  expect(fontAvailable).toBe(true);
});

test("no console errors or failed requests during navigation", async ({ page }) => {
  const consoleErrors: string[] = [];
  const failedRequests: string[] = [];

  page.on("console", (msg) => {
    if (msg.type() === "error") {
      consoleErrors.push(msg.text());
    }
  });
  page.on("requestfailed", (request) => {
    if (request.failure()?.errorText !== "net::ERR_ABORTED") {
      failedRequests.push(request.url());
    }
  });

  await mockAuthenticatedAPI(page, [account]);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/");
  await page.getByRole("link", { name: "Transfer", exact: true }).click();
  await page.goto(`/accounts/${account.id}`);

  // Wait for loaded activity state before checking listener arrays
  await expect(page.getByText("−$25.00")).toBeVisible();
  await expect(page.getByText("Sent")).toBeVisible();

  expect(consoleErrors).toEqual([]);
  expect(failedRequests).toEqual([]);
});

test("dashboard screenshots remain stable at 320 and 1440 with populated data", async ({
  page,
}) => {
  await mockAuthenticatedAPI(page, [account]);

  for (const viewport of [
    { width: 320, height: 800 },
    { width: 1440, height: 1000 },
  ]) {
    await page.setViewportSize(viewport);
    await page.goto("/");
    await expect(page.getByRole("heading", { name: /Good to see you/ })).toBeVisible();

    // Wait for populated account card markers (Copy/Activity) before screenshot
    await expect(page.getByRole("button", { name: /copy account number/i })).toBeVisible();
    await expect(page.getByRole("link", { name: /activity/i })).toBeVisible();

    // Wait for fonts to be ready and stable layout signals
    await page.evaluate(() => document.fonts.ready);

    await expect(page).toHaveScreenshot(`dashboard-${viewport.width}.png`, {
      animations: "disabled",
      fullPage: true,
    });
  }
});
