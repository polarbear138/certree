import React from "react";
import { afterEach, beforeEach, describe, it, expect, vi } from "vitest";
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
import { HistoryPage, Login, TreeExplorer } from "./App";
import type { TreeNode } from "./api";
const n = (
  id: string,
  name: string,
  type: string,
  children: TreeNode[] = [],
): TreeNode => ({
  id,
  entity_id: id,
  name,
  type,
  status: "Unknown",
  counts: { Unknown: 1 },
  children,
});
const tree = n("root", "Certree", "root", [
  n("o", "Example University", "organization", [
    n("e", "Production", "environment"),
  ]),
]);
function mount(component: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <BrowserRouter>{component}</BrowserRouter>
    </QueryClientProvider>,
  );
}
beforeEach(() => {
  localStorage.clear();
  // JSDOM does not implement browser scrolling; emulate its observable position.
  Object.defineProperty(HTMLElement.prototype, "scrollTo", {
    configurable: true,
    value: function (options: ScrollToOptions) {
      this.scrollTop = options.top || 0;
      this.dispatchEvent(new Event("scroll"));
    },
  });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
describe("administrator console interactions", () => {
  it("reports rejected login without accepting an identity", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ error: "invalid credentials" }), {
            status: 401,
          }),
      ),
    );
    const onLogin = vi.fn();
    mount(<Login onLogin={onLogin} />);
    fireEvent.change(screen.getByLabelText("Username"), {
      target: { value: "admin" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "wrong" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "invalid credentials",
    );
    expect(onLogin).not.toHaveBeenCalled();
  });
  it("supports tree arrow navigation, expansion, and search empty state", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async (url: string) =>
          new Response(JSON.stringify(url === "/api/tree" ? tree : []), {
            status: 200,
          }),
      ),
    );
    mount(<TreeExplorer />);
    await screen.findByRole("tree");
    const root = screen.getByRole("treeitem", { name: /Certree/ });
    root.focus();
    fireEvent.keyDown(root, { key: "ArrowDown" });
    const org = screen.getByRole("treeitem", { name: /Example University/ });
    await waitFor(() => expect(org).toHaveAttribute("aria-selected", "true"));
    fireEvent.keyDown(org, { key: "ArrowRight" });
    expect(org).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("treeitem", { name: /Production/ })).toBeVisible();
    fireEvent.keyDown(org, { key: "Home" });
    expect(root).toHaveAttribute("aria-selected", "true");
    fireEvent.change(screen.getByLabelText("Search tree"), {
      target: { value: "missing.invalid" },
    });
    expect(
      screen.getByRole("heading", { name: "No matching nodes" }),
    ).toBeVisible();
  });
});

describe("server history", () => {
  it("defaults to verification, preserves server selection between tabs, and closes stale details", async () => {
    window.history.replaceState({}, "", "/history");
    const fetchMock = vi.fn(async (url: string) => {
      const parsed = new URL(url, "http://localhost");
      let data: unknown = [];
      if (parsed.pathname === "/api/servers")
        data = [
          { id: "s1", name: "Nginx 01" },
          { id: "s2", name: "Nginx 02" },
        ];
      if (parsed.pathname === "/api/deployments") {
        const server = parsed.searchParams.get("server_id") || "s1";
        const kind = parsed.searchParams.get("kind");
        data = [
          {
            id: `${server}-${kind}`,
            kind,
            server_id: server,
            server_name: server === "s1" ? "Nginx 01" : "Nginx 02",
            sni: "lab.test",
            status: "Success",
            stage: "Success",
            created_at: "2026-10-24T00:00:00Z",
          },
        ];
      } else if (parsed.pathname.startsWith("/api/deployments/"))
        data = {
          job: { kind: "Verification", status: "Success" },
          results: [],
          verifications: [],
        };
      return new Response(JSON.stringify(data), { status: 200 });
    });
    vi.stubGlobal("fetch", fetchMock);
    mount(<HistoryPage />);
    expect(screen.getByRole("tab", { name: "Verification" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await screen.findByRole("button", { name: "s1-Verific" });
    fireEvent.click(screen.getByRole("button", { name: "s1-Verific" }));
    await screen.findByRole("heading", { name: /Verification job/ });
    fireEvent.change(screen.getByRole("combobox", { name: "Server" }), {
      target: { value: "s2" },
    });
    await screen.findByRole("button", { name: "s2-Verific" });
    expect(
      screen.queryByRole("heading", { name: /Verification job/ }),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: "Deployments" }));
    await screen.findByRole("button", { name: "s2-Deploym" });
    expect(screen.getByRole("combobox", { name: "Server" })).toHaveValue("s2");
    expect(
      screen.queryByRole("button", { name: "s2-Verific" }),
    ).not.toBeInTheDocument();
    fireEvent.keyDown(screen.getByRole("tab", { name: "Deployments" }), {
      key: "ArrowLeft",
    });
    await screen.findByRole("button", { name: "s2-Verific" });
    expect(screen.getByRole("tab", { name: "Verification" })).toHaveFocus();
    expect(
      fetchMock.mock.calls.some(([url]) =>
        url.includes("kind=Deployment&server_id=s2"),
      ),
    ).toBe(true);
    window.history.replaceState({}, "", "/");
  });
});
