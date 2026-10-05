import { describe, it, expect } from "vitest";
import { filterTree, flatten, TreeNode } from "./api";
const node = (
  id: string,
  name: string,
  status: string,
  children: TreeNode[] = [],
): TreeNode => ({
  id,
  entity_id: id,
  name,
  status,
  type: "certificate",
  counts: {},
  children,
});
const tree = node("root", "Certree", "Failed", [
  node("org", "University", "Failed", [
    node("cert", "*.example.com", "Failed", [
      node("t1", "Nginx 01", "Healthy"),
      node("t2", "Nginx 02", "Failed"),
    ]),
  ]),
]);
describe("tree navigation and filtering", () => {
  it("filters status leaves while preserving their ancestry", () => {
    const v = filterTree(tree, "", "Failed");
    expect(v?.children[0].children[0].children.map((n) => n.id)).toEqual([
      "t2",
    ]);
  });
  it("keeps the ancestry of matching domains", () => {
    const v = filterTree(tree, "example.com", "");
    expect(v?.children[0].children[0].id).toBe("cert");
    expect(v?.children[0].children[0].children).toHaveLength(2);
  });
  it("keeps only failed leaves when a name and status are combined", () => {
    const v = filterTree(tree, "Nginx", "Failed");
    expect(v?.children[0].children[0].children.map((n) => n.id)).toEqual([
      "t2",
    ]);
  });
  it("uses stable entity identifiers across collapse and expand", () => {
    expect(flatten(tree, new Set(["root"])).map((n) => n.node.id)).toEqual([
      "root",
      "org",
    ]);
    expect(
      flatten(tree, new Set(["root", "org", "cert"])).map((n) => n.depth),
    ).toEqual([1, 2, 3, 4, 4]);
  });
  it("returns an empty result for an unknown domain", () =>
    expect(filterTree(tree, "missing.invalid", "")).toBeNull());
});
