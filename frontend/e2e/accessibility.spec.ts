import { AxeBuilder } from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

const viewports = [
  { width: 320, height: 800 },
  { width: 768, height: 900 },
  { width: 1024, height: 900 },
  { width: 1440, height: 1000 },
];

const user = {
  username: "alexandria",
  full_name: "Alexandria Montgomery-Worthington Alexandria Montgomery-Worthington",
  email: "alexandria@example.com",
  is_email_verified: true,
  created_at: "2026-01-01T00:00:00Z",
};

const account = {
  id: "11111111-2222-3333-4444-555566667777",
  owner: user.username,
  balance: 125_000,
  currency: "USD",
  created_at: "2026-01-15T10:00:00Z",
};

async function mockAuthenticatedAPI(page: Page, accounts: unknown[] = []): Promise<void> {
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
  // Register specific account transfer route before broad account routes
  await page.route(`**/api/v1/accounts/${account.id}/transfers?*`, (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify([
        {
          id: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
          from_account_id: account.id,
          to_account_id: "99999999-8888-7777-6666-555544443333",
          amount: 25_00,
          idempotency_key: "11111111-aaaa-bbbb-cccc-222222222222",
          created_at: "2026-01-16T10:00:00Z",
        },
      ]),
    }),
  );
  await page.route(`**/api/v1/accounts/${account.id}`, (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(account) }),
  );
  await page.route("**/api/v1/accounts?*", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(accounts) }),
  );
  await page.route("**/api/v1/transfer-limits", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: "{}" }),
  );
}

async function expectNoAccessibilityViolations(page: Page): Promise<void> {
  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
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
  await page.route("**/api/v1/accounts?*", (route) =>
    route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ error: "Service unavailable" }),
    }),
  );
  await page.setViewportSize({ width: 320, height: 800 });
  await page.goto("/transfer");

  await expect(page.getByRole("alert")).toContainText(
    "Couldn't load your accounts. Service unavailable",
  );
  await expect(page.getByRole("button", { name: "Retry" })).toBeVisible();
  await expectNoAccessibilityViolations(page);
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
    failedRequests.push(request.url());
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
