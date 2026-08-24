import { expect, test } from "@playwright/test";
import { apiErrors } from "./support/mock-api.js";

test("login explains when another attempt is allowed", async ({ page }) => {
  await page.route("**/api/v1/tokens/renew", (route) => route.fulfill({ status: 204 }));
  await page.route("**/api/v1/users/login", (route) =>
    route.fulfill({
      status: 429,
      contentType: "application/json",
      headers: { "Retry-After": "5" },
      body: JSON.stringify(apiErrors.rateLimited),
    }),
  );

  await page.goto("/login");
  await page.getByRole("textbox", { name: "Username" }).fill("alexandria");
  await page.getByLabel("Password").fill("incorrect password");
  await page.getByRole("button", { name: "Sign in" }).click();

  await expect(page.getByRole("alert")).toHaveText("Too many attempts. Try again in 5 seconds.");
  await expect(page.getByRole("button", { name: "Sign in" })).toBeEnabled();
});
