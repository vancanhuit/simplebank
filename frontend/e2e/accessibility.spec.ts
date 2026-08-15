import AxeBuilder from "@axe-core/playwright";
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
      await menu.click();
      await expect(page.getByRole("navigation", { name: "Mobile primary" })).toBeVisible();
      await expect(page.getByRole("link", { name: "Transfer", exact: true }).last()).toBeVisible();
      await page.getByRole("link", { name: "Overview", exact: true }).last().click();
      await expect(page.getByRole("navigation", { name: "Mobile primary" })).toBeHidden();
      await expect(page.getByRole("button", { name: "Open navigation" })).toBeFocused();
    } else {
      await expect(page.getByRole("navigation", { name: "Primary", exact: true })).toBeVisible();
    }

    await expectNoAccessibilityViolations(page);
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
