import { test, expect } from "@playwright/test";
import { resolve } from "node:path";
test("administrator creates a trust tree, uploads, approves, and inspects real data", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await page.getByLabel("Username").fill("admin");
  await page
    .getByLabel("Password", { exact: true })
    .fill("test-admin-password-2026");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(
    page.getByRole("heading", { name: "Certificate overview" }),
  ).toBeVisible();
  await page
    .getByRole("link", { name: "Tree Explorer", exact: false })
    .first()
    .click();
  await page.getByRole("button", { name: "Organization", exact: true }).click();
  await page.getByLabel("Organization name").fill("Example University");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await page.getByRole("button", { name: "Environment", exact: true }).click();
  await page
    .getByRole("combobox", { name: "Organization", exact: true })
    .selectOption({ label: "Example University" });
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await page.getByRole("link", { name: "Certificates", exact: true }).click();
  await page.getByRole("button", { name: "Upload certificate" }).click();
  await page
    .getByRole("combobox", { name: "Environment", exact: true })
    .selectOption({ label: "Production" });
  await page
    .getByLabel("Certificate and private key files")
    .setInputFiles(resolve("../../.cache/lab/v1.pem"));
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await expect(
    page.getByRole("cell", { name: "Awaiting approval" }),
  ).toBeVisible();
  await page
    .getByRole("link", { name: "Tree Explorer", exact: false })
    .first()
    .click();
  await page.getByLabel("Search tree").fill("lab.test");
  await page
    .getByRole("treeitem")
    .filter({ hasText: "*.example.test, lab.test" })
    .click();
  await expect(
    page.getByRole("heading", { name: "Certificate information", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Approve version" }).click();
  await expect(page.getByText("Approved", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Toggle theme" }).click();
  await page.screenshot({
    path: resolve("../../docs/screenshots/tree-dark.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "Toggle theme" }).click();
  await page.screenshot({
    path: resolve("../../docs/screenshots/tree-light.png"),
    fullPage: true,
  });
  await page.getByLabel("Search tree").fill("missing.invalid");
  await expect(
    page.getByRole("heading", { name: "No matching nodes" }),
  ).toBeVisible();
  await page.getByLabel("Search tree").fill("");
  await page.getByLabel("Filter status").selectOption("Failed");
  await expect(
    page.getByRole("heading", { name: "No matching nodes" }),
  ).toBeVisible();
  await page.getByLabel("Filter status").selectOption("");
  await page.getByRole("link", { name: "Settings", exact: true }).click();
  await page.getByLabel("Warning window (days)").fill("45");
  await page.getByRole("button", { name: "Save settings" }).click();
  await expect(page.getByText("Warning window saved.")).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({
    path: resolve("../../docs/screenshots/mobile.png"),
    fullPage: true,
  });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.getByRole("button", { name: "Log out" }).click();
  await expect(
    page.getByRole("heading", { name: "Welcome back" }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});
