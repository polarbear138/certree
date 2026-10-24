import { test, expect } from "@playwright/test";

test("tree bulk review uses exact versions and displays applied expiry", async ({
  page,
}) => {
  const expiry = (days: number) => ({
    state: days <= 30 ? "Warning" : "Healthy",
    not_before: "2026-07-01T00:00:00Z",
    not_after: days <= 30 ? "2026-11-05T00:00:00Z" : "2027-01-01T00:00:00Z",
    days_remaining: days,
    remaining_percent: days <= 30 ? 10 : 65,
    total: 1,
    known: 1,
    unknown: 0,
    not_deployed: 0,
  });
  const target = (id: string, name: string, days: number) => ({
    id,
    entity_id: id,
    type: "target",
    name,
    status: days <= 30 ? "Warning" : "Healthy",
    counts: { Healthy: 1 },
    children: [],
    data: {
      server_id: id,
      server_name: name.split(" · ")[0],
      expiry: expiry(days),
      server_expiry: expiry(days),
    },
  });
  const tree = {
    id: "root",
    entity_id: "root",
    type: "root",
    name: "Certree",
    status: "Warning",
    counts: { Healthy: 1, Warning: 1 },
    children: [
      {
        id: "org",
        entity_id: "org",
        type: "organization",
        name: "Example University",
        status: "Warning",
        counts: { Healthy: 1, Warning: 1 },
        children: [
          {
            id: "env",
            entity_id: "env",
            type: "environment",
            name: "Production",
            status: "Warning",
            counts: { Healthy: 1, Warning: 1 },
            children: [
              {
                id: "cert",
                entity_id: "cert",
                type: "certificate",
                name: "lab.test",
                status: "Warning",
                counts: { Healthy: 1, Warning: 1 },
                children: [
                  target("t1", "Nginx 01 · lab.test", 12),
                  target("t2", "Nginx 02 · lab.test", 69),
                ],
              },
            ],
          },
        ],
      },
    ],
  };
  await page.addInitScript(() =>
    localStorage.setItem(
      "tree-expanded",
      JSON.stringify(["root", "org", "env", "cert"]),
    ),
  );
  await page.route("**/api/**", (route) =>
    route.fulfill({
      json: route.request().url().endsWith("/auth/me")
        ? { User: "admin", CSRF: "test" }
        : route.request().url().endsWith("/tree")
          ? tree
          : route.request().url().endsWith("/tree/summary")
            ? { counts: {} }
            : [],
    }),
  );
  await page.route("**/api/deployments/bulk/preview", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            target_id: "t1",
            version_id: "version-reviewed-1",
            server_name: "Nginx 01",
            certificate_name: "lab.test",
            sni: "lab.test",
            fingerprint: "1234567890abcdef".repeat(4),
            not_after: "2027-01-01T00:00:00Z",
            ready: true,
          },
          {
            target_id: "t2",
            server_name: "Nginx 02",
            certificate_name: "lab.test",
            sni: "lab.test",
            ready: false,
            error: "No approved certificate version",
          },
        ],
      },
    }),
  );
  await page.route("**/api/deployments/bulk", async (route) => {
    expect(route.request().postDataJSON()).toEqual({
      items: [{ target_id: "t1", version_id: "version-reviewed-1" }],
    });
    await route.fulfill({
      json: {
        items: [
          {
            target_id: "t1",
            version_id: "version-reviewed-1",
            server_name: "Nginx 01",
            ready: true,
            job_id: "queued-job-1",
          },
        ],
      },
    });
  });
  await page.goto("/tree");
  await expect(page.getByText("12d left", { exact: true })).toBeVisible();
  await expect(page.getByText("69d left", { exact: true })).toBeVisible();
  await page
    .getByRole("checkbox", { name: "Select Nginx 01 · lab.test", exact: true })
    .check();
  await expect(
    page.getByRole("checkbox", { name: "Select lab.test", exact: true }),
  ).toBeChecked({ indeterminate: true });
  await page
    .getByRole("checkbox", { name: "Select Nginx 02 · lab.test", exact: true })
    .check();
  await page.screenshot({
    path: "../../artifacts/tree-expiry-selection.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "Review deployment (2)" }).click();
  await expect(
    page.getByText("No approved certificate version", { exact: true }),
  ).toBeVisible();
  await page.screenshot({
    path: "../../artifacts/tree-bulk-review.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "Deploy 1 ready targets" }).click();
  await expect(page.getByRole("status")).toContainText("1 queued");
  await page.getByRole("button", { name: "Close", exact: true }).last().click();
  await expect(
    page.getByRole("button", { name: "Review deployment (1)" }),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({
    path: "../../artifacts/tree-operations-mobile.png",
    fullPage: true,
  });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
});
