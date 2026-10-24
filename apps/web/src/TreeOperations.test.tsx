import React from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router-dom";
import { TreeExplorer } from "./App";
import { ExpiryBar } from "./TreeOperations";
import type { TreeNode } from "./api";

beforeEach(() => {
  localStorage.clear();
  localStorage.setItem("tree-expanded", JSON.stringify(["root", "cert"]));
  Object.defineProperty(HTMLDialogElement.prototype, "showModal", {
    configurable: true,
    value: function () {
      this.open = true;
    },
  });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("selects filtered descendants, retains hidden selections and submits only reviewed ready versions", async () => {
  const leaf = (id: string, name: string): TreeNode => ({
    id,
    entity_id: id,
    type: "target",
    name,
    status: "Healthy",
    counts: { Healthy: 1 },
    children: [],
    data: {
      expiry: {
        state: "Healthy",
        not_after: "2027-01-01T00:00:00Z",
        days_remaining: 69,
        remaining_percent: 50,
      },
    },
  });
  const tree: TreeNode = {
    id: "root",
    entity_id: "root",
    type: "root",
    name: "Certree",
    status: "Healthy",
    counts: { Healthy: 2 },
    children: [
      {
        id: "cert",
        entity_id: "cert",
        type: "certificate",
        name: "lab.test",
        status: "Healthy",
        counts: { Healthy: 2 },
        children: [leaf("t1", "Nginx 01"), leaf("t2", "Nginx 02")],
      },
    ],
  };
  const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
    let data: unknown = [];
    if (url === "/api/tree") data = tree;
    if (url === "/api/deployments/bulk/preview")
      data = {
        items: [
          {
            target_id: "t1",
            version_id: "approved-v1",
            server_name: "Nginx 01",
            ready: true,
            fingerprint: "abc",
          },
          {
            target_id: "t2",
            ready: false,
            error: "No approved certificate version",
          },
        ],
      };
    if (url === "/api/deployments/bulk") {
      expect(JSON.parse(init?.body as string)).toEqual({
        items: [{ target_id: "t1", version_id: "approved-v1" }],
      });
      data = {
        items: [
          {
            target_id: "t1",
            version_id: "approved-v1",
            job_id: "job1",
            ready: true,
          },
        ],
      };
    }
    return new Response(JSON.stringify(data), { status: 200 });
  });
  vi.stubGlobal("fetch", fetchMock);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <BrowserRouter>
        <TreeExplorer />
      </BrowserRouter>
    </QueryClientProvider>,
  );
  fireEvent.click(
    await screen.findByRole("checkbox", { name: "Select Nginx 01" }),
  );
  expect(
    screen.getByRole("checkbox", { name: "Select lab.test" }),
  ).toBePartiallyChecked();
  fireEvent.change(screen.getByLabelText("Search tree"), {
    target: { value: "Nginx 02" },
  });
  expect(
    screen.getByText(/includes targets outside current filter/),
  ).toBeVisible();
  fireEvent.click(
    screen.getByRole("button", { name: "Select matching targets" }),
  );
  fireEvent.click(
    screen.getByRole("button", { name: "Review deployment (2)" }),
  );
  expect(
    await screen.findByText("No approved certificate version"),
  ).toBeVisible();
  fireEvent.click(
    screen.getByRole("button", { name: "Deploy 1 ready targets" }),
  );
  await screen.findByText(/1 queued/);
  fireEvent.click(
    screen.getAllByRole("button", { name: "Close" }).at(-1)!,
  );
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "Review deployment (1)" }),
    ).toBeVisible(),
  );
  expect(
    screen.getByRole("checkbox", { name: "Select Nginx 02" }),
  ).toBeChecked();
  const previewCall = fetchMock.mock.calls.find(
    ([url]) => url === "/api/deployments/bulk/preview",
  );
  expect(JSON.parse(previewCall?.[1]?.body as string)).toEqual({
    target_ids: ["t1", "t2"],
  });
});

it("shows remaining lifetime without inventing progress for undeployed targets", () => {
  const { rerender } = render(
    <ExpiryBar
      value={{
        state: "Warning",
        not_after: "2026-11-03T00:00:00Z",
        days_remaining: 10,
        remaining_percent: 10,
      }}
    />,
  );
  expect(screen.getByText("10d left")).toBeVisible();
  expect(screen.getByRole("progressbar")).toHaveAttribute("value", "10");
  rerender(<ExpiryBar value={{ state: "Not deployed" }} />);
  expect(screen.getByText("Not deployed")).toBeVisible();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  rerender(
    <ExpiryBar
      value={{
        state: "Expired",
        not_after: "2026-01-01T00:00:00Z",
        days_remaining: -30,
        remaining_percent: 0,
      }}
    />,
  );
  expect(screen.getByText("Expired")).toBeVisible();
});
