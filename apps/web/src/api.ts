export type Row = Record<string, any>;
export type TreeNode = {
  id: string;
  entity_id: string;
  type: string;
  name: string;
  status: string;
  counts: Record<string, number>;
  children: TreeNode[];
  data?: Row;
};
let csrf = "";
export function setCSRF(v: string) {
  csrf = v;
}
export async function api<T = Row>(
  path: string,
  body?: object | FormData,
  method?: string,
): Promise<T> {
  const form = body instanceof FormData;
  const res = await fetch("/api/" + path, {
    method: method || (body ? "POST" : "GET"),
    credentials: "same-origin",
    headers: {
      ...(body && !form ? { "Content-Type": "application/json" } : {}),
      ...(body || method ? { "X-CSRF-Token": csrf } : {}),
    },
    body: body ? (form ? body : JSON.stringify(body)) : undefined,
  });
  const json = await res.json();
  if (res.status === 401 && !path.endsWith("login"))
    window.dispatchEvent(new Event("certree:unauthorized"));
  if (!res.ok) throw new Error(json.error || "Request failed");
  return json;
}
export function unpack(v: any) {
  if (typeof v === "string") {
    try {
      return JSON.parse(v);
    } catch {
      return v;
    }
  }
  return v;
}
export function flatten(
  n: TreeNode,
  expanded: Set<string>,
  depth = 1,
): { node: TreeNode; depth: number }[] {
  return [
    { node: n, depth },
    ...(expanded.has(n.id)
      ? n.children.flatMap((c) => flatten(c, expanded, depth + 1))
      : []),
  ];
}
export function filterTree(
  n: TreeNode,
  q: string,
  status: string,
): TreeNode | null {
  const children = n.children
    .map((c) => filterTree(c, q, status))
    .filter((c): c is TreeNode => !!c);
  const match =
    (!q || n.name.toLowerCase().includes(q.toLowerCase())) &&
    (!status || n.status === status);
  return match && !status
    ? n
    : match || children.length
      ? { ...n, children }
      : null;
}
export function findNode(n: TreeNode, id: string): TreeNode | undefined {
  if (n.id === id) return n;
  for (const c of n.children) {
    const v = findNode(c, id);
    if (v) return v;
  }
}
