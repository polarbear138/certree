import React, { useEffect, useRef, useState } from "react";

import {
  QueryClient,
  QueryClientProvider,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { BrowserRouter, NavLink, useLocation } from "react-router-dom";
import {
  TreePine,
  LayoutDashboard,
  Network,
  ShieldCheck,
  Server,
  Rocket,
  Inbox,
  Settings,
  Search,
  Sun,
  Moon,
  LogOut,
  Plus,
  ChevronRight,
  ChevronDown,
  CheckCircle2,
  AlertTriangle,
  XCircle,
  Clock,
  HelpCircle,
  Loader2,
  ArrowUpRight,
  Upload,
  X,
  RefreshCw,
  Leaf,
  Folder,
  Building2,
} from "lucide-react";
import {
  api,
  findNode,
  flatten,
  filterTree,
  Row,
  setCSRF,
  TreeNode,
  unpack,
} from "./api";
import "./styles.css";
export const qc = new QueryClient({
  defaultOptions: { queries: { retry: false, refetchOnWindowFocus: true } },
});
const statusIcons: Record<string, React.ElementType> = {
  Healthy: CheckCircle2,
  Success: CheckCircle2,
  Warning: AlertTriangle,
  Failed: XCircle,
  "Recovery Failed": XCircle,
  "Rolled Back": AlertTriangle,
  Pending: Clock,
  Deploying: Loader2,
  Running: Loader2,
  Unknown: HelpCircle,
};
function Status({ value }: { value: string }) {
  const Icon = statusIcons[value] || HelpCircle;
  return (
    <span className={"badge status-" + value.replaceAll(" ", "-")}>
      <Icon size={13} />
      {value}
    </span>
  );
}
const date = (v: any) =>
  v
    ? new Date(v).toLocaleDateString(undefined, {
        month: "short",
        day: "numeric",
        year: "numeric",
      })
    : "—";
function useRows(path: string) {
  return useQuery({
    queryKey: [path],
    queryFn: () => api<Row[]>(path),
    refetchInterval: 5000,
  });
}
function Notice({ error }: { error: Error | null | undefined }) {
  return error ? (
    <div role="alert" className="notice error">
      {error.message}
    </div>
  ) : null;
}
function Empty({
  title,
  children,
}: {
  title: string;
  children?: React.ReactNode;
}) {
  return (
    <div className="empty">
      <TreePine size={32} />
      <h3>{title}</h3>
      <p>{children || "New records will appear here once you add them."}</p>
    </div>
  );
}
type Field = {
  key: string;
  label: string;
  type?: string;
  options?: { id: string; name: string }[];
  value?: string;
  hint?: string;
  optional?: boolean;
};
function FormModal({
  title,
  fields,
  onClose,
  onSubmit,
}: {
  title: string;
  fields: Field[];
  onClose: () => void;
  onSubmit: (data: Row) => Promise<unknown>;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const mutation = useMutation({
    mutationFn: onSubmit,
    onSuccess: () => {
      qc.invalidateQueries();
      onClose();
    },
  });
  useEffect(() => {
    ref.current?.showModal();
  }, []);
  return (
    <dialog ref={ref} onCancel={onClose} className="modal">
      <div className="modal-head">
        <h2>{title}</h2>
        <button aria-label="Close" onClick={onClose}>
          <X size={18} />
        </button>
      </div>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          const fd = new FormData(e.currentTarget);
          const d: Row = {};
          for (const f of fields) {
            d[f.key] =
              f.type === "number"
                ? Number(fd.get(f.key))
                : f.type === "checkbox"
                  ? fd.has(f.key)
                  : f.type === "file"
                    ? fd.getAll(f.key)
                    : fd.get(f.key);
          }
          mutation.mutate(d);
        }}
      >
        <div className="form-fields">
          {fields.map((f) => (
            <label key={f.key}>
              {f.label}
              {f.options ? (
                <select
                  name={f.key}
                  required={!f.optional}
                  defaultValue={f.value || ""}
                >
                  <option value="">Select…</option>
                  {f.options.map((o) => (
                    <option key={o.id} value={o.id}>
                      {o.name}
                    </option>
                  ))}
                </select>
              ) : f.type === "textarea" ? (
                <textarea
                  name={f.key}
                  required={!f.optional}
                  rows={4}
                  spellCheck={false}
                />
              ) : (
                <input
                  name={f.key}
                  type={f.type || "text"}
                  required={!f.optional && f.type !== "checkbox"}
                  defaultValue={f.value}
                  multiple={f.type === "file"}
                  autoComplete={f.type === "password" ? "new-password" : "off"}
                />
              )}{" "}
              {f.hint && <small>{f.hint}</small>}
            </label>
          ))}
        </div>
        <Notice error={mutation.error} />
        <footer>
          <button type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="primary" disabled={mutation.isPending}>
            {mutation.isPending ? <Loader2 size={15} /> : <Plus size={15} />}
            Save
          </button>
        </footer>
      </form>
    </dialog>
  );
}
export function Login({ onLogin }: { onLogin: (v: Row) => void }) {
  const m = useMutation({
    mutationFn: (d: Row) => api("auth/login", d),
    onSuccess: onLogin,
  });
  return (
    <div className="login">
      <div className="login-brand">
        <TreePine size={44} />
        <span>CERTREE</span>
        <h1>
          Trust, from
          <br />
          root to leaf.
        </h1>
        <p>
          TLS certificate lifecycle management.
          <br />
          Collect. Deploy. Verify.
        </p>
        <div className="orbit">
          <TreePine size={120} />
        </div>
        <small>OPEN SOURCE · APACHE 2.0</small>
      </div>
      <form
        className="login-form"
        onSubmit={(e) => {
          e.preventDefault();
          const f = new FormData(e.currentTarget);
          m.mutate({
            username: f.get("username"),
            password: f.get("password"),
          });
        }}
      >
        <span className="eyebrow">CERTREE CONSOLE</span>
        <h2>Welcome back</h2>
        <p>Sign in to manage your certificate infrastructure.</p>
        <label>
          Username
          <input name="username" autoComplete="username" required autoFocus />
        </label>
        <label>
          Password
          <input
            name="password"
            type="password"
            autoComplete="current-password"
            required
          />
        </label>
        <Notice error={m.error} />
        <button className="primary" disabled={m.isPending}>
          {m.isPending ? "Signing in…" : "Sign in"}
          <ArrowUpRight size={18} />
        </button>
        <small>
          Use the administrator account configured during installation.
        </small>
      </form>
    </div>
  );
}
const nav = [
  ["/", "Overview", LayoutDashboard],
  ["/tree", "Tree Explorer", Network],
  ["/certificates", "Certificates", ShieldCheck],
  ["/servers", "Servers", Server],
  ["/deployments", "Deployments", Rocket],
  ["/collectors", "Collectors", Inbox],
  ["/settings", "Settings", Settings],
] as const;
export function App() {
  const [user, setUser] = useState<Row | null>(null);
  const [boot, setBoot] = useState(true);
  const [dark, setDark] = useState(localStorage.getItem("theme") === "dark");
  const location = useLocation();
  useEffect(() => {
    const expired = () => {
      setCSRF("");
      setUser(null);
      qc.clear();
    };
    window.addEventListener("certree:unauthorized", expired);
    return () => window.removeEventListener("certree:unauthorized", expired);
  }, []);
  useEffect(() => {
    api("auth/me")
      .then((v) => {
        setCSRF(v.CSRF);
        setUser(v);
      })
      .catch(() => {})
      .finally(() => setBoot(false));
  }, []);
  useEffect(() => {
    document.documentElement.dataset.theme = dark ? "dark" : "light";
    localStorage.setItem("theme", dark ? "dark" : "light");
  }, [dark]);
  if (boot)
    return (
      <div className="loading">
        <Loader2 />
        Loading Certree…
      </div>
    );
  if (!user)
    return (
      <Login
        onLogin={(v) => {
          setCSRF(v.CSRF);
          setUser(v);
        }}
      />
    );
  const page = nav.find((n) => n[0] === location.pathname)?.[1] || "Overview";
  return (
    <div className="shell">
      <aside className="sidebar">
        <NavLink to="/" className="brand">
          <div>
            <TreePine size={25} />
          </div>
          CERTREE
        </NavLink>
        <div className="workspace">
          <span className="workspace-icon">C</span>
          <div>
            Infrastructure<small>Administrator workspace</small>
          </div>
          <span className="live-dot" />
        </div>
        <span className="nav-caption">WORKSPACE</span>
        <nav>
          {nav.map(([path, label, Icon]) => (
            <NavLink key={path} to={path} end={path === "/"}>
              <Icon size={18} />
              {label}
              {label === "Tree Explorer" && (
                <span className="new-pill">CORE</span>
              )}
            </NavLink>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="trust-card">
            <Leaf size={19} />
            <strong>Root to leaf.</strong>
            <p>
              Deployment is complete when
              <br />
              live TLS is verified.
            </p>
          </div>
          <div className="user">
            <span className="avatar">
              {user.User?.slice(0, 1).toUpperCase()}
            </span>
            <div>
              {user.User}
              <small>Administrator</small>
            </div>
            <button
              aria-label="Log out"
              onClick={() =>
                api("auth/logout", {}).then(() => {
                  qc.clear();
                  setUser(null);
                })
              }
            >
              <LogOut size={17} />
            </button>
          </div>
        </div>
      </aside>
      <div className="content">
        <header className="topbar">
          <span>
            Workspace <ChevronRight size={14} /> <b>{page}</b>
          </span>
          <div>
            <span className="secure">
              <ShieldCheck size={14} /> Secure console
            </span>
            <button aria-label="Toggle theme" onClick={() => setDark(!dark)}>
              {dark ? <Sun size={18} /> : <Moon size={18} />}
            </button>
            <button
              className="mobile-logout"
              aria-label="Log out"
              onClick={() =>
                api("auth/logout", {}).then(() => {
                  qc.clear();
                  setUser(null);
                })
              }
            >
              <LogOut size={17} />
            </button>
            <span className="avatar small">
              {user.User?.slice(0, 1).toUpperCase()}
            </span>
          </div>
        </header>
        <main>
          {location.pathname === "/tree" ? (
            <TreeExplorer />
          ) : location.pathname === "/certificates" ? (
            <Certificates />
          ) : location.pathname === "/servers" ? (
            <Servers />
          ) : location.pathname === "/deployments" ? (
            <Deployments />
          ) : location.pathname === "/collectors" ? (
            <Collectors />
          ) : location.pathname === "/settings" ? (
            <SettingsPage />
          ) : (
            <Overview />
          )}
        </main>
        <footer className="page-footer">
          Certree v0.1.0 <span>TLS Certificate Management, Root to Leaf.</span>
        </footer>
      </div>
    </div>
  );
}
function Heading({
  eyebrow,
  title,
  description,
  children,
}: {
  eyebrow: string;
  title: string;
  description: string;
  children?: React.ReactNode;
}) {
  return (
    <div className="heading">
      <div>
        <span className="eyebrow">{eyebrow}</span>
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      <div className="actions">{children}</div>
    </div>
  );
}
function Overview() {
  const summary = useQuery({
    queryKey: ["summary"],
    queryFn: () => api("tree/summary"),
    refetchInterval: 5000,
  });
  const jobs = useRows("deployments");
  const certs = useRows("certificates");
  const data = summary.data;
  const expiring = (certs.data || []).filter((c) => {
    const m = unpack(c.metadata);
    return new Date(m.not_after).getTime() - Date.now() < 30 * 86400000;
  });
  return (
    <>
      <Heading
        eyebrow="INFRASTRUCTURE AT A GLANCE"
        title="Certificate overview"
        description="A clear view of your certificates, deployments, and live TLS health."
      >
        <NavLink className="button primary" to="/tree">
          <Network size={16} />
          Open Tree Explorer
        </NavLink>
      </Heading>
      <Notice error={summary.error} />
      <div className="metrics">
        {[
          [
            "Certificates",
            data?.certificates,
            ShieldCheck,
            "Across all environments",
          ],
          [
            "Deployment targets",
            data?.targets,
            Server,
            "Registered service endpoints",
          ],
          [
            "Expiring within 30 days",
            expiring.length,
            Clock,
            "Renew before interruption",
          ],
          [
            "Failed targets",
            data?.counts?.Failed || 0,
            AlertTriangle,
            "Requires your attention",
          ],
        ].map(([label, value, Icon, caption]) => {
          const I = Icon as React.ElementType;
          return (
            <div className="metric" key={label as string}>
              <div>
                <span>{label as string}</span>
                <I size={18} />
              </div>
              <strong>{value === undefined ? "—" : String(value)}</strong>
              <small>{caption as string}</small>
            </div>
          );
        })}
      </div>
      <div className="overview-grid">
        <section className="panel">
          <div className="panel-title">
            <h2>Recent deployments</h2>
            <NavLink to="/deployments">
              View all <ArrowUpRight size={14} />
            </NavLink>
          </div>
          <Notice error={jobs.error} />
          <JobTable rows={(jobs.data || []).slice(0, 5)} />
        </section>
        <section className="panel">
          <div className="panel-title">
            <h2>Lifecycle health</h2>
            <Status value={data?.status || "Unknown"} />
          </div>
          <div className="health-list">
            {["Healthy", "Warning", "Failed", "Pending", "Unknown"].map((s) => (
              <div key={s}>
                <Status value={s} />
                <strong>{data?.counts?.[s] || 0}</strong>
              </div>
            ))}
          </div>
          <small className="panel-note">
            Counts include target leaves and certificate warnings.
          </small>
        </section>
      </div>
      <section className="panel">
        <div className="panel-title">
          <h2>Expiration watch</h2>
          <span className="subtle">30-day window</span>
        </div>
        {expiring.length ? (
          <div className="cert-grid">
            {expiring.map((c) => (
              <div key={c.id}>
                <ShieldCheck size={20} />
                <strong>{c.name}</strong>
                <small>Expires {date(unpack(c.metadata).not_after)}</small>
                <Status value="Warning" />
              </div>
            ))}
          </div>
        ) : (
          <Empty title="No certificates approaching expiration">
            Upload your certificate inventory to monitor its lifecycle.
          </Empty>
        )}
      </section>
      <div className="onboarding">
        <div>
          <span className="eyebrow">YOUR FIRST CERTIFICATE</span>
          <h2>Build your trust tree</h2>
          <p>
            Create an organization and environment, upload a bundle, then
            connect a target.
          </p>
        </div>
        <NavLink to="/tree" className="button">
          Get started <ArrowUpRight size={16} />
        </NavLink>
      </div>
    </>
  );
}
export function TreeExplorer() {
  const query = useQuery({
    queryKey: ["tree"],
    queryFn: () => api<TreeNode>("tree"),
    refetchInterval: 3000,
  });
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");
  const [selected, setSelected] = useState("root");
  const [expanded, setExpanded] = useState<Set<string>>(
    () =>
      new Set(JSON.parse(localStorage.getItem("tree-expanded") || '["root"]')),
  );
  const [modal, setModal] = useState("");
  const orgs = useRows("organizations");
  const tree = query.data;
  const filtered = tree ? filterTree(tree, search, status) : null;
  const allExpanded = new Set<string>();
  function gather(n: TreeNode) {
    allExpanded.add(n.id);
    n.children.forEach(gather);
  }
  if (filtered && (search || status)) gather(filtered);
  const visible = filtered
    ? flatten(filtered, search || status ? allExpanded : expanded)
    : [];
  const current = tree ? findNode(tree, selected) : undefined;
  const [scroll, setScroll] = useState(0);
  const viewport = useRef<HTMLDivElement>(null);
  const toggle = (id: string) => {
    const next = new Set(expanded);
    next.has(id) ? next.delete(id) : next.add(id);
    setExpanded(next);
    localStorage.setItem("tree-expanded", JSON.stringify([...next]));
  };
  const start = Math.max(0, Math.floor(scroll / 46) - 5);
  const end = Math.min(visible.length, start + 30);
  return (
    <>
      <Heading
        eyebrow="CERTIFICATE RELATIONSHIPS"
        title="Tree Explorer"
        description="From organization to endpoint. Every certificate, every connection."
      >
        <button onClick={() => setModal("organization")}>
          <Plus size={16} />
          Organization
        </button>
        <button className="primary" onClick={() => setModal("environment")}>
          <Plus size={16} />
          Environment
        </button>
      </Heading>
      <div className="tree-toolbar">
        <div className="search">
          <Search size={17} />
          <input
            aria-label="Search tree"
            placeholder="Search names or domains…"
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setScroll(0);
            }}
          />
        </div>
        <select
          aria-label="Filter status"
          value={status}
          onChange={(e) => {
            setStatus(e.target.value);
            setScroll(0);
          }}
        >
          <option value="">All statuses</option>
          {[
            "Healthy",
            "Warning",
            "Failed",
            "Pending",
            "Deploying",
            "Unknown",
          ].map((s) => (
            <option key={s}>{s}</option>
          ))}
        </select>
        <span className="subtle">
          <span className="live-dot" />
          Updates every 3s
        </span>
      </div>
      <Notice error={query.error} />
      {query.isPending ? (
        <div className="loading">Loading relationships…</div>
      ) : (
        <div className="tree-layout">
          <section className="panel tree-panel">
            <div className="panel-title">
              <h2>Trust tree</h2>
              <span className="subtle">{visible.length} visible nodes</span>
            </div>
            <div
              className="tree-viewport"
              role="tree"
              aria-label="Certificate infrastructure"
              ref={viewport}
              onScroll={(e) => setScroll(e.currentTarget.scrollTop)}
              style={{ height: 540 }}
            >
              {visible.length === 0 ? (
                <Empty title="No matching nodes" />
              ) : (
                <div
                  style={{ height: visible.length * 46, position: "relative" }}
                >
                  {visible
                    .slice(start, end)
                    .map(({ node: n, depth }, index) => {
                      const i = start + index;
                      const Icon =
                        n.type === "certificate"
                          ? ShieldCheck
                          : n.type === "target"
                            ? Server
                            : n.type === "organization"
                              ? Building2
                              : n.type === "root"
                                ? TreePine
                                : Folder;
                      return (
                        <div
                          key={n.id}
                          role="treeitem"
                          aria-level={depth}
                          aria-selected={n.id === selected}
                          aria-expanded={
                            n.children.length ? expanded.has(n.id) : undefined
                          }
                          tabIndex={
                            n.id === selected ||
                            (!visible.some((v) => v.node.id === selected) &&
                              i === 0)
                              ? 0
                              : -1
                          }
                          className={
                            "tree-row " + (n.id === selected ? "selected" : "")
                          }
                          style={{
                            top: i * 46,
                            height: 46,
                            paddingLeft: 12 + (depth - 1) * 20,
                          }}
                          onClick={() => setSelected(n.id)}
                          onKeyDown={(e) => {
                            let next = i;
                            if (e.key === "ArrowDown")
                              next = Math.min(i + 1, visible.length - 1);
                            else if (e.key === "ArrowUp")
                              next = Math.max(i - 1, 0);
                            else if (e.key === "Home") next = 0;
                            else if (e.key === "End") next = visible.length - 1;
                            else if (e.key === "ArrowRight") {
                              if (n.children.length && !expanded.has(n.id))
                                toggle(n.id);
                              else next = Math.min(i + 1, visible.length - 1);
                            } else if (e.key === "ArrowLeft") {
                              if (expanded.has(n.id)) toggle(n.id);
                              else {
                                for (let k = i - 1; k >= 0; k--) {
                                  if (visible[k].depth < depth) {
                                    next = k;
                                    break;
                                  }
                                }
                              }
                            } else if (e.key === "Enter" || e.key === " ") {
                              toggle(n.id);
                            } else return;
                            e.preventDefault();
                            if (next !== i) {
                              setSelected(visible[next].node.id);
                              viewport.current?.scrollTo({
                                top: Math.max(0, next * 46 - 150),
                              });
                              setTimeout(
                                () =>
                                  viewport.current
                                    ?.querySelector<HTMLElement>(
                                      '[aria-selected="true"]',
                                    )
                                    ?.focus(),
                                0,
                              );
                            }
                          }}
                        >
                          <button
                            tabIndex={-1}
                            className="expand"
                            aria-label={
                              expanded.has(n.id)
                                ? "Collapse " + n.name
                                : "Expand " + n.name
                            }
                            onClick={(e) => {
                              e.stopPropagation();
                              toggle(n.id);
                            }}
                          >
                            {n.children.length ? (
                              expanded.has(n.id) || search || status ? (
                                <ChevronDown size={14} />
                              ) : (
                                <ChevronRight size={14} />
                              )
                            ) : (
                              <span />
                            )}
                          </button>
                          <Icon size={17} />
                          <span className="node-name" title={n.name}>
                            {n.name}
                          </span>
                          {n.children.length > 0 && (
                            <small className="child-count">
                              {n.children.length}
                            </small>
                          )}
                          <span
                            className={"status-dot status-" + n.status}
                            title={n.status}
                            aria-label={n.status}
                          />
                        </div>
                      );
                    })}
                </div>
              )}
            </div>
            <div className="tree-legend">
              <Status value="Healthy" />
              <Status value="Warning" />
              <Status value="Failed" />
            </div>
          </section>
          <section className="panel detail-panel">
            {current ? (
              <NodeDetails node={current} />
            ) : (
              <Empty title="Select a node" />
            )}
          </section>
        </div>
      )}
      {modal && (
        <FormModal
          title={"Create " + modal}
          fields={
            modal === "organization"
              ? [{ key: "name", label: "Organization name" }]
              : [
                  {
                    key: "organization_id",
                    label: "Organization",
                    options: (orgs.data as any) || [],
                  },
                  {
                    key: "name",
                    label: "Environment name",
                    value: "Production",
                  },
                ]
          }
          onClose={() => setModal("")}
          onSubmit={(d) =>
            api(modal === "organization" ? "organizations" : "environments", d)
          }
        />
      )}
    </>
  );
}
function NodeDetails({ node: n }: { node: TreeNode }) {
  const cert = useQuery({
    queryKey: ["certificate", n.entity_id],
    queryFn: () => api("certificates/" + n.entity_id),
    enabled: n.type === "certificate",
    refetchInterval: 5000,
  });
  const jobs = useRows("deployments");
  const verifications = useRows("verifications");
  const m = useMutation({
    mutationFn: ({ path, data }: { path: string; data: Row }) =>
      api(path, data),
    onSuccess: () => qc.invalidateQueries(),
  });
  const [version, setVersion] = useState("");
  const [compare, setCompare] = useState("");
  const [targetVersion, setTargetVersion] = useState("");
  const targets = useRows("targets");
  const targetCert = useQuery({
    queryKey: ["target-certificate", n.data?.certificate_id],
    queryFn: () => api("certificates/" + n.data?.certificate_id),
    enabled: n.type === "target",
    refetchInterval: 5000,
  });
  const c = n.type === "certificate" ? cert.data : targetCert.data;
  const versions: Row[] = c?.versions || [];
  const chosen =
    versions.find(
      (v) => v.id === (n.type === "target" ? targetVersion : version),
    ) || versions[0];
  const earlier = versions.find((v) => v.id === compare);
  const meta = unpack(chosen?.metadata || n.data?.metadata || {});
  const v = unpack(chosen?.validation || n.data?.validation || {});
  const relatedJobs = (jobs.data || []).filter((j) =>
    n.type === "target"
      ? j.target_id === n.entity_id
      : (targets.data || []).some(
          (t) => t.certificate_id === n.entity_id && t.id === j.target_id,
        ),
  );
  const live = (verifications.data || []).find(
    (x) => x.target_id === n.entity_id,
  );
  useEffect(() => {
    setVersion("");
    setCompare("");
    setTargetVersion("");
  }, [n.id]);
  return (
    <>
      <div className="detail-head">
        <span className="eyebrow">{n.type.toUpperCase()} NODE</span>
        <h2>{n.name}</h2>
        <Status value={n.status} />
      </div>
      <Notice error={m.error || cert.error || targetCert.error} />
      {m.isSuccess && (
        <div className="notice">
          Request saved. Live status will refresh automatically.
        </div>
      )}
      {n.type === "certificate" || n.type === "target" ? (
        <>
          <div className="detail-section">
            <h3>
              {n.type === "certificate"
                ? "Certificate information"
                : "Service endpoint"}
            </h3>
            {n.type === "target" && (
              <dl>
                <dt>Verification host</dt>
                <dd>
                  {n.data?.host}:{n.data?.port}
                </dd>
                <dt>SNI</dt>
                <dd>{n.data?.sni}</dd>
                <dt>Deployment</dt>
                <dd>
                  <Status value={n.data?.deployment_status} />
                </dd>
                <dt>Live verification</dt>
                <dd>
                  <Status value={n.data?.verification_status} />
                </dd>
              </dl>
            )}
            {versions.length > 0 && (
              <label>
                Certificate version
                <select
                  value={chosen?.id || ""}
                  onChange={(e) =>
                    n.type === "target"
                      ? setTargetVersion(e.target.value)
                      : setVersion(e.target.value)
                  }
                >
                  {versions.map((x, i) => (
                    <option key={x.id} value={x.id}>
                      {i === 0 ? "Latest · " : ""}
                      {date(x.created_at)} · {x.fingerprint.slice(0, 12)}
                    </option>
                  ))}
                </select>
              </label>
            )}
            <dl>
              <dt>Domains</dt>
              <dd>{meta.sans?.join(", ") || "—"}</dd>
              <dt>Issuer</dt>
              <dd>{meta.issuer || "—"}</dd>
              <dt>Valid until</dt>
              <dd>{date(meta.not_after)}</dd>
              <dt>Validation</dt>
              <dd>
                <Status value={v.valid ? "Healthy" : "Failed"} />
              </dd>
              <dt>Approval</dt>
              <dd>
                {chosen?.approved
                  ? "Approved"
                  : "Administrator approval required"}
              </dd>
              <dt>SHA-256</dt>
              <dd className="mono wrap">{meta.fingerprint || "—"}</dd>
            </dl>
            {v.errors?.length > 0 && (
              <div className="notice error">{v.errors.join("; ")}</div>
            )}
            {n.type === "certificate" && (
              <button
                className="primary"
                disabled={
                  !chosen || !v.valid || !!chosen.approved || m.isPending
                }
                onClick={() =>
                  m.mutate({
                    path: "certificates/versions/" + chosen.id + "/approve",
                    data: {},
                  })
                }
              >
                <ShieldCheck size={15} />
                Approve version
              </button>
            )}
          </div>
          {n.type === "certificate" && versions.length > 1 && (
            <div className="detail-section">
              <h3>Compare versions</h3>
              <select
                aria-label="Compare version"
                value={compare}
                onChange={(e) => setCompare(e.target.value)}
              >
                <option value="">Choose a comparison version…</option>
                {versions
                  .filter((x) => x.id !== chosen?.id)
                  .map((x) => (
                    <option key={x.id} value={x.id}>
                      {date(x.created_at)} · {x.fingerprint.slice(0, 12)}
                    </option>
                  ))}
              </select>
              {earlier && (
                <table>
                  <thead>
                    <tr>
                      <th>Field</th>
                      <th>Selected</th>
                      <th>Comparison</th>
                    </tr>
                  </thead>
                  <tbody>
                    {[
                      "not_after",
                      "issuer",
                      "public_key_algorithm",
                      "fingerprint",
                    ].map((key) => (
                      <tr key={key}>
                        <td>{key.replaceAll("_", " ")}</td>
                        <td className="wrap">{String(meta[key])}</td>
                        <td className="wrap">
                          {String(unpack(earlier.metadata)[key])}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          )}
          {n.type === "certificate" && (
            <div className="detail-section">
              <h3>Deployment targets</h3>
              {n.children.length ? (
                n.children.map((t) => (
                  <div className="target-action" key={t.id}>
                    <div>
                      <strong>{t.name}</strong>
                      <Status value={t.status} />
                    </div>
                    <button
                      disabled={!chosen?.approved || m.isPending}
                      onClick={() =>
                        m.mutate({
                          path: "deployments",
                          data: {
                            target_id: t.entity_id,
                            version_id: chosen?.id,
                            kind: "Deployment",
                          },
                        })
                      }
                    >
                      <Rocket size={14} />
                      Deploy
                    </button>
                  </div>
                ))
              ) : (
                <p className="subtle">
                  Add an endpoint in Servers to deploy this certificate.
                </p>
              )}
            </div>
          )}
          {n.type === "target" && (
            <>
              <div className="detail-section action-row">
                <button
                  disabled={!chosen?.approved || m.isPending}
                  className="primary"
                  onClick={() =>
                    m.mutate({
                      path: "deployments",
                      data: {
                        target_id: n.entity_id,
                        version_id: chosen?.id,
                        kind: "Deployment",
                      },
                    })
                  }
                >
                  <Rocket size={15} />
                  Deploy selected version
                </button>
                <button
                  disabled={!chosen?.approved || m.isPending}
                  onClick={() =>
                    m.mutate({
                      path: "deployments",
                      data: {
                        target_id: n.entity_id,
                        version_id: chosen?.id,
                        kind: "Verification",
                      },
                    })
                  }
                >
                  <ShieldCheck size={15} />
                  Verify TLS
                </button>
                <button
                  disabled={m.isPending}
                  onClick={() =>
                    m.mutate({
                      path: "servers/" + n.data?.server_id + "/test",
                      data: {},
                    })
                  }
                >
                  <Network size={15} />
                  Test SSH
                </button>
                <small>
                  Choose an older approved version to restore it through the
                  same validated deployment process.
                </small>
              </div>
              <div className="detail-section">
                <h3>Latest live TLS result</h3>
                {live ? (
                  <dl>
                    <dt>Checked</dt>
                    <dd>{date(live.created_at)}</dd>
                    <dt>Expected</dt>
                    <dd className="mono wrap">{live.expected}</dd>
                    <dt>Served</dt>
                    <dd className="mono wrap">
                      {live.actual || "No trusted handshake"}
                    </dd>
                    <dt>Result</dt>
                    <dd>
                      {unpack(live.result).healthy
                        ? "Verified"
                        : unpack(live.result).error}
                    </dd>
                  </dl>
                ) : (
                  <p className="subtle">No live TLS verification recorded.</p>
                )}
              </div>
            </>
          )}
          <div className="detail-section">
            <h3>Deployment history</h3>
            <JobTable rows={relatedJobs.slice(0, 8)} />
          </div>
        </>
      ) : (
        <>
          <div className="detail-section">
            <h3>Descendant health</h3>
            <div className="health-list">
              {Object.entries(n.counts).map(([s, count]) => (
                <div key={s}>
                  <Status value={s} />
                  <strong>{count}</strong>
                </div>
              ))}
            </div>
          </div>
          <div className="detail-section">
            <h3>Deployment preview</h3>
            <p>
              {n.children.length} immediate child nodes. Select a certificate or
              endpoint to review and approve an individual deployment.
            </p>
            <p className="subtle">
              Organization and environment deployment previews are read only in
              v0.1.
            </p>
            <ul className="plain-list">
              {n.children.map((c) => (
                <li key={c.id}>
                  <span>{c.name}</span>
                  <Status value={c.status} />
                </li>
              ))}
            </ul>
          </div>
        </>
      )}
    </>
  );
}
function Certificates() {
  const rows = useRows("certificates");
  const envs = useRows("environments");
  const [upload, setUpload] = useState(false);
  return (
    <>
      <Heading
        eyebrow="CERTIFICATE INVENTORY"
        title="Certificates"
        description="Validated bundles, immutable versions, and explicit deployment approval."
      >
        <button className="primary" onClick={() => setUpload(true)}>
          <Upload size={16} />
          Upload certificate
        </button>
      </Heading>
      <Notice error={rows.error} />
      <section className="panel">
        {rows.isPending ? (
          <div className="loading">Loading certificates…</div>
        ) : !rows.data?.length ? (
          <Empty title="Your certificate inventory starts here">
            Upload PEM + KEY files, a ZIP bundle, or a PKCS#12 archive.
          </Empty>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Certificate / domains</th>
                <th>Issuer</th>
                <th>Expires</th>
                <th>Validation</th>
                <th>Approval</th>
              </tr>
            </thead>
            <tbody>
              {rows.data.map((c) => (
                <tr key={c.id}>
                  <td>
                    <NavLink to="/tree">
                      <ShieldCheck size={17} />
                      {c.name}
                    </NavLink>
                    <small className="mono">
                      {c.fingerprint.slice(0, 20)}…
                    </small>
                  </td>
                  <td>{unpack(c.metadata).issuer}</td>
                  <td>{date(unpack(c.metadata).not_after)}</td>
                  <td>
                    <Status
                      value={unpack(c.validation).valid ? "Healthy" : "Failed"}
                    />
                  </td>
                  <td>{c.approved ? "Approved" : "Awaiting approval"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
      {upload && (
        <FormModal
          title="Upload certificate bundle"
          fields={[
            {
              key: "environment_id",
              label: "Environment",
              options: (envs.data as any) || [],
            },
            {
              key: "files",
              label: "Certificate and private key files",
              type: "file",
              hint: "PEM, DER, PFX/P12 or ZIP. Maximum total size 16 MiB.",
            },
            {
              key: "password",
              label: "PKCS#12 password",
              type: "password",
              optional: true,
              hint: "Used for import only; never stored.",
            },
          ]}
          onClose={() => setUpload(false)}
          onSubmit={(d) => {
            const f = new FormData();
            f.set("environment_id", d.environment_id);
            f.set("password", d.password || "");
            for (const file of d.files) f.append("files", file);
            return api("certificates", f);
          }}
        />
      )}
    </>
  );
}
function Servers() {
  const rows = useRows("servers");
  const certs = useRows("certificates");
  const targets = useRows("targets");
  const [modal, setModal] = useState("");
  const m = useMutation({
    mutationFn: (id: string) => api("servers/" + id + "/test", {}),
    onSuccess: () => qc.invalidateQueries(),
  });
  const serverFields: Field[] = [
    { key: "name", label: "Server name" },
    { key: "host", label: "SSH host / IP" },
    { key: "port", label: "SSH port", type: "number", value: "22" },
    { key: "user", label: "SSH username" },
    {
      key: "type",
      label: "Server type",
      options: [{ id: "nginx", name: "Nginx" }],
      value: "nginx",
    },
    {
      key: "host_key",
      label: "Pinned OpenSSH host public key",
      type: "textarea",
      hint: "Verify this key through a trusted channel before registering.",
    },
    {
      key: "ssh_key",
      label: "SSH private key",
      type: "textarea",
      hint: "Encrypted at rest. It is never returned by the API.",
    },
  ];
  const targetFields: Field[] = [
    { key: "server_id", label: "Server", options: (rows.data as any) || [] },
    {
      key: "certificate_id",
      label: "Certificate",
      options: (certs.data as any) || [],
    },
    {
      key: "host",
      label: "Live TLS verification host",
      hint: "May be a load balancer or proxy address.",
    },
    { key: "port", label: "TLS port", type: "number", value: "443" },
    { key: "sni", label: "Service domain / SNI" },
    {
      key: "certificate_path",
      label: "Remote full-chain certificate path",
      value: "/etc/nginx/tls/fullchain.pem",
    },
    {
      key: "key_path",
      label: "Remote private key path",
      value: "/etc/nginx/tls/private.key",
    },
    {
      key: "configuration_path",
      label: "Nginx main configuration path",
      value: "/etc/nginx/nginx.conf",
    },
    {
      key: "backup_path",
      label: "Restricted backup directory",
      value: "/var/lib/certree/backups",
    },
    {
      key: "sudo",
      label: "Use narrowly scoped sudo for nginx test / reload",
      type: "checkbox",
      optional: true,
    },
  ];
  return (
    <>
      <Heading
        eyebrow="DEPLOYMENT INVENTORY"
        title="Servers & endpoints"
        description="Pinned SSH identities and independent TLS verification endpoints."
      >
        <button onClick={() => setModal("target")}>
          <Plus size={16} />
          Add target
        </button>
        <button className="primary" onClick={() => setModal("server")}>
          <Plus size={16} />
          Register server
        </button>
      </Heading>
      <Notice error={rows.error || m.error} />
      {m.isSuccess && (
        <div className="notice">
          SSH authentication, host key verification and SFTP connection
          succeeded.
        </div>
      )}
      <section className="panel">
        <div className="panel-title">
          <h2>Registered servers</h2>
          <span className="subtle">{rows.data?.length || 0} servers</span>
        </div>
        {!rows.data?.length ? (
          <Empty title="Connect your first Nginx server" />
        ) : (
          <table>
            <thead>
              <tr>
                <th>Server</th>
                <th>SSH address</th>
                <th>User</th>
                <th>Host identity</th>
                <th>Action</th>
              </tr>
            </thead>
            <tbody>
              {rows.data.map((s) => {
                const c = unpack(s.config);
                return (
                  <tr key={s.id}>
                    <td>
                      <strong>{s.name}</strong>
                      <small>Nginx</small>
                    </td>
                    <td>
                      {c.host}:{c.port}
                    </td>
                    <td>{c.user}</td>
                    <td>
                      <ShieldCheck size={15} />
                      Pinned key
                    </td>
                    <td>
                      <button
                        disabled={m.isPending}
                        onClick={() => m.mutate(s.id)}
                      >
                        <Network size={14} />
                        Test connection
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </section>
      <section className="panel">
        <div className="panel-title">
          <h2>Deployment targets</h2>
        </div>
        {!targets.data?.length ? (
          <Empty title="No deployment targets">
            Link a certificate to a registered server and a service endpoint.
          </Empty>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Endpoint</th>
                <th>Certificate</th>
                <th>Deployment</th>
                <th>Live TLS</th>
              </tr>
            </thead>
            <tbody>
              {targets.data.map((t) => (
                <tr key={t.id}>
                  <td>
                    {t.sni}
                    <small>
                      {t.host}:{t.port}
                    </small>
                  </td>
                  <td>
                    {certs.data?.find((c) => c.id === t.certificate_id)?.name}
                  </td>
                  <td>
                    <Status value={t.deployment_status} />
                  </td>
                  <td>
                    <Status value={t.verification_status} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
      {modal && (
        <FormModal
          title={
            modal === "server"
              ? "Register Nginx server"
              : "Create deployment target"
          }
          fields={modal === "server" ? serverFields : targetFields}
          onClose={() => setModal("")}
          onSubmit={(d) => api(modal === "server" ? "servers" : "targets", d)}
        />
      )}
    </>
  );
}
function JobTable({
  rows,
  onSelect,
}: {
  rows: Row[];
  onSelect?: (id: string) => void;
}) {
  return rows.length ? (
    <table>
      <thead>
        <tr>
          <th>Job</th>
          <th>Status</th>
          <th>Stage</th>
          <th>Created</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((j) => (
          <tr key={j.id}>
            <td>
              {onSelect ? (
                <button
                  className="text-button mono"
                  onClick={() => onSelect(j.id)}
                >
                  {j.id.slice(0, 10)}
                </button>
              ) : (
                <span className="mono">{j.id.slice(0, 10)}</span>
              )}
              <small>{j.kind}</small>
            </td>
            <td>
              <Status value={j.status} />
            </td>
            <td>
              {j.stage}
              <small className="error-text">{j.error}</small>
            </td>
            <td>{date(j.created_at)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  ) : (
    <Empty title="No deployments yet">
      Approve a certificate version and deploy it from Tree Explorer.
    </Empty>
  );
}
function Deployments() {
  const rows = useRows("deployments");
  const cancellation = useMutation({
    mutationFn: (id: string) => api("deployments/" + id + "/cancel", {}),
    onSuccess: () => qc.invalidateQueries(),
  });
  const [selected, setSelected] = useState("");
  const detail = useQuery({
    queryKey: ["job", selected],
    queryFn: () => api("deployments/" + selected),
    enabled: !!selected,
    refetchInterval: 2000,
  });
  return (
    <>
      <Heading
        eyebrow="AUDITABLE OPERATIONS"
        title="Deployments"
        description="Persistent jobs, live verification, and recovery outcomes."
      >
        <button onClick={() => qc.invalidateQueries()}>
          <RefreshCw size={16} />
          Refresh
        </button>
      </Heading>
      <Notice error={rows.error || detail.error || cancellation.error} />
      <section className="panel">
        <JobTable rows={rows.data || []} onSelect={setSelected} />
      </section>
      {detail.data && (
        <section className="panel">
          <div className="panel-title">
            <h2>Job {selected.slice(0, 12)}</h2>
            <Status value={detail.data.job.status} />
            {detail.data.job.status === "Pending" && (
              <button
                disabled={cancellation.isPending}
                onClick={() => cancellation.mutate(selected)}
              >
                Cancel pending job
              </button>
            )}
          </div>
          <div className="timeline">
            {detail.data.results.map((r: Row) => (
              <div key={r.id}>
                <CheckCircle2 size={16} />
                <strong>{r.outcome}</strong>
                <span>{r.detail}</span>
                <small>{new Date(r.created_at).toLocaleTimeString()}</small>
              </div>
            ))}
          </div>
          {detail.data.verifications.map((v: Row) => (
            <div className="detail-section" key={v.id}>
              <strong>Live TLS verification</strong>
              <dl>
                <dt>Expected</dt>
                <dd className="mono wrap">{v.expected}</dd>
                <dt>Actual</dt>
                <dd className="mono wrap">
                  {v.actual || "No trusted handshake"}
                </dd>
                <dt>Outcome</dt>
                <dd>
                  {unpack(v.result).healthy
                    ? "Verified"
                    : unpack(v.result).error}
                </dd>
              </dl>
            </div>
          ))}
        </section>
      )}
    </>
  );
}
function Collectors() {
  const rows = useRows("collectors");
  const events = useRows("collector-events");
  const envs = useRows("environments");
  const [modal, setModal] = useState(false);
  const m = useMutation({
    mutationFn: ({ id, action }: { id: string; action: string }) =>
      api("collectors/" + id + "/" + action, {}),
    onSuccess: () => qc.invalidateQueries(),
  });
  return (
    <>
      <Heading
        eyebrow="CERTIFICATE COLLECTION"
        title="Collectors"
        description="Collect email attachments over verified TLS. Administrator approval always applies."
      >
        <button className="primary" onClick={() => setModal(true)}>
          <Plus size={16} />
          Add IMAP collector
        </button>
      </Heading>
      <Notice error={rows.error || events.error || m.error} />
      {m.isSuccess && (
        <div className="notice">
          {m.data?.connected
            ? "IMAP TLS connection, authentication and mailbox check succeeded."
            : "Collection queued; results appear after the next worker poll."}
        </div>
      )}
      <section className="panel">
        {!rows.data?.length ? (
          <Empty title="Collect certificates from your inbox">
            Configure a dedicated IMAP mailbox with sender and subject filters.
          </Empty>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Collector</th>
                <th>Mailbox</th>
                <th>Last poll</th>
                <th>Status</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {rows.data.map((c) => {
                const cfg = unpack(c.config);
                return (
                  <tr key={c.id}>
                    <td>
                      {c.name}
                      <small>{cfg.host}</small>
                    </td>
                    <td>{cfg.mailbox}</td>
                    <td>
                      {c.last_poll ? date(c.last_poll * 1000) : "Not polled"}
                    </td>
                    <td>{c.last_error || "Ready"}</td>
                    <td className="action-row">
                      <button
                        disabled={m.isPending}
                        onClick={() => m.mutate({ id: c.id, action: "test" })}
                      >
                        Test
                      </button>
                      <button
                        disabled={m.isPending}
                        onClick={() => m.mutate({ id: c.id, action: "poll" })}
                      >
                        Collect now
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </section>
      <section className="panel">
        <div className="panel-title">
          <h2>Collection history</h2>
        </div>
        {!events.data?.length ? (
          <Empty title="No collection events" />
        ) : (
          <table>
            <thead>
              <tr>
                <th>Message UID</th>
                <th>Result</th>
                <th>Time</th>
              </tr>
            </thead>
            <tbody>
              {events.data.map((e) => (
                <tr key={e.id}>
                  <td className="mono">{e.message_key}</td>
                  <td>{e.result}</td>
                  <td>{date(e.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
      {modal && (
        <FormModal
          title="Add IMAP collector"
          fields={[
            { key: "name", label: "Collector name" },
            {
              key: "environment_id",
              label: "Environment",
              options: (envs.data as any) || [],
            },
            { key: "host", label: "IMAP TLS hostname" },
            {
              key: "port",
              label: "IMAP TLS port",
              type: "number",
              value: "993",
            },
            { key: "username", label: "IMAP username" },
            {
              key: "password",
              label: "IMAP password / application token",
              type: "password",
            },
            { key: "mailbox", label: "Mailbox", value: "INBOX" },
            { key: "from", label: "Sender filter", optional: true },
            { key: "subject", label: "Subject filter", optional: true },
            {
              key: "interval",
              label: "Polling interval (seconds)",
              type: "number",
              value: "300",
            },
          ]}
          onClose={() => setModal(false)}
          onSubmit={(d) => api("collectors", d)}
        />
      )}
    </>
  );
}
function SettingsPage() {
  const [passwordModal, setPasswordModal] = useState(false);
  const settings = useRows("settings");
  const audit = useRows("audit-logs");
  const m = useMutation({
    mutationFn: (days: number) => api("settings", { expiry_days: days }, "PUT"),
    onSuccess: () => qc.invalidateQueries(),
  });
  return (
    <>
      <Heading
        eyebrow="SYSTEM CONFIGURATION"
        title="Settings"
        description="Lifecycle thresholds, security posture, and audit history."
      />
      <section className="panel settings-panel">
        <h2>Certificate expiration warning</h2>
        <p>Mark certificates with a warning before they expire.</p>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            m.mutate(Number(new FormData(e.currentTarget).get("days")));
          }}
        >
          <label>
            Warning window (days)
            <input
              key={settings.data?.[0]?.value}
              name="days"
              type="number"
              min="1"
              max="365"
              required
              defaultValue={
                settings.data?.find((x) => x.key === "expiry_days")?.value || 30
              }
            />
          </label>
          <button className="primary" disabled={m.isPending}>
            Save settings
          </button>
        </form>
        <Notice error={m.error} />
        {m.isSuccess && <div className="notice">Warning window saved.</div>}
      </section>
      <section className="panel settings-panel">
        <h2>Security & system</h2>
        <dl>
          <dt>Version</dt>
          <dd>0.1.0</dd>
          <dt>Secret storage</dt>
          <dd>AES-256-GCM; master key outside SQLite</dd>
          <dt>Authentication</dt>
          <dd>bcrypt · HttpOnly sessions · CSRF protection</dd>
          <dt>SSH trust</dt>
          <dd>Pinned host keys required</dd>
          <dt>Deployment policy</dt>
          <dd>Approved versions only; live TLS verification required</dd>
          <dt>Administrator account</dt>
          <dd>
            Configured at installation. Changing your password revokes all
            sessions.
          </dd>
        </dl>
        <button onClick={() => setPasswordModal(true)}>
          Change administrator password
        </button>
      </section>
      {passwordModal && (
        <FormModal
          title="Change password"
          fields={[
            {
              key: "current_password",
              label: "Current password",
              type: "password",
            },
            {
              key: "new_password",
              label: "New password",
              type: "password",
              hint: "14–72 bytes. Sign in again after saving.",
            },
          ]}
          onClose={() => setPasswordModal(false)}
          onSubmit={(d) =>
            api("auth/password", d).then(() =>
              window.dispatchEvent(new Event("certree:unauthorized")),
            )
          }
        />
      )}
      <section className="panel">
        <div className="panel-title">
          <h2>Audit log</h2>
        </div>
        <Notice error={audit.error} />
        {!audit.data?.length ? (
          <Empty title="No audit events" />
        ) : (
          <table>
            <thead>
              <tr>
                <th>Actor</th>
                <th>Action</th>
                <th>Entity</th>
                <th>Time</th>
              </tr>
            </thead>
            <tbody>
              {audit.data.map((a) => (
                <tr key={a.id}>
                  <td>{a.actor}</td>
                  <td>{a.action}</td>
                  <td className="mono">{a.entity.slice(0, 16) || "—"}</td>
                  <td>{date(a.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </>
  );
}
