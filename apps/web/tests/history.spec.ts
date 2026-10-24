import { test, expect } from "@playwright/test";

test("history tabs and server links keep verification separate from deployments", async ({
  page,
}) => {
  await page.route("**/api/**", (route) =>
    route.fulfill({
      json: route.request().url().endsWith("/auth/me")
        ? { User: "admin", CSRF: "test" }
        : route.request().url().endsWith("/tree/summary")
          ? { counts: {} }
          : [],
    }),
  );
  await page.route("**/api/servers", (route) =>
    route.fulfill({
      json: [
        {
          id: "s1",
          name: "Nginx 01",
          config: JSON.stringify({
            host: "10.0.0.1",
            port: 22,
            user: "certree",
          }),
        },
        {
          id: "s2",
          name: "Nginx 02",
          config: JSON.stringify({
            host: "10.0.0.2",
            port: 22,
            user: "certree",
          }),
        },
      ],
    }),
  );
  await page.route("**/api/deployments?*", (route) => {
    const params = new URL(route.request().url()).searchParams;
    const kind = params.get("kind");
    const server = params.get("server_id") || "s1";
    return route.fulfill({
      json: [
        {
          id: `${server}-${kind}`,
          server_id: server,
          server_name: server === "s1" ? "Nginx 01" : "Nginx 02",
          kind,
          sni: "lab.test",
          port: 443,
          status: "Success",
          stage: "Success",
          created_at: "2026-10-24T11:00:00Z",
        },
      ],
    });
  });
  await page.goto("/");
  await page.getByRole("link", { name: "History", exact: true }).click();
  await expect(
    page.getByRole("tab", { name: "Verification", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(page.getByRole("button", { name: "s1-Verific" })).toBeVisible();
  await page
    .getByRole("combobox", { name: "Server", exact: true })
    .selectOption("s2");
  await expect(page.getByRole("button", { name: "s2-Verific" })).toBeVisible();
  await page.screenshot({
    path: "../../artifacts/history-verification.png",
    fullPage: true,
  });
  await page.getByRole("tab", { name: "Deployments", exact: true }).click();
  await expect(page.getByRole("button", { name: "s2-Deploym" })).toBeVisible();
  await expect(
    page.getByRole("combobox", { name: "Server", exact: true }),
  ).toHaveValue("s2");
  await page.reload();
  await expect(
    page.getByRole("tab", { name: "Deployments", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await page.getByRole("link", { name: "Servers", exact: true }).click();
  await page
    .getByRole("row")
    .filter({ hasText: "Nginx 02" })
    .getByRole("link", { name: "History" })
    .click();
  await expect(
    page.getByRole("tab", { name: "Verification", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(
    page.getByRole("combobox", { name: "Server", exact: true }),
  ).toHaveValue("s2");
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({
    path: "../../artifacts/history-mobile.png",
    fullPage: true,
  });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
});
