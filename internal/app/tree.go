package app

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Node struct {
	ID       string         `json:"id"`
	EntityID string         `json:"entity_id"`
	Type     string         `json:"type"`
	Name     string         `json:"name"`
	Status   string         `json:"status"`
	Counts   map[string]int `json:"counts"`
	Children []*Node        `json:"children"`
	Data     map[string]any `json:"data,omitempty"`
}

var statuses = []string{"Failed", "Deploying", "Pending", "Warning", "Unknown", "Healthy"}

func node(id, entity, kind, name, status string) *Node {
	return &Node{id, entity, kind, name, status, map[string]int{}, []*Node{}, nil}
}
func aggregate(n *Node) {
	for _, c := range n.Children {
		aggregate(c)
		for k, v := range c.Counts {
			n.Counts[k] += v
		}
	}
	if len(n.Children) == 0 {
		n.Counts[n.Status]++
		return
	}
	if n.Type == "certificate" && n.Status == "Warning" {
		n.Counts["Warning"]++
	}
	if n.Type == "certificate" && n.Status == "Failed" {
		n.Counts["Failed"]++
	}
	if n.Type == "certificate" && n.Status == "Pending" {
		n.Counts["Pending"]++
	}
	n.Status = "Healthy"
	for _, s := range statuses {
		if n.Counts[s] > 0 {
			n.Status = s
			break
		}
	}
}
func (a *App) BuildTree() (*Node, error) {
	root := node("root", "root", "root", "Certree", "Unknown")
	orgs, e := a.Store.Rows("SELECT * FROM organizations ORDER BY name")
	if e != nil {
		return nil, e
	}
	envs, e := a.Store.Rows("SELECT * FROM environments ORDER BY name")
	if e != nil {
		return nil, e
	}
	certs, e := a.Store.Rows(`SELECT c.*,v.id version_id,v.metadata,v.validation,v.approved FROM certificates c JOIN certificate_versions v ON v.id=(SELECT id FROM certificate_versions WHERE certificate_id=c.id ORDER BY created_at DESC LIMIT 1) ORDER BY c.name`)
	if e != nil {
		return nil, e
	}
	targets, e := a.Store.Rows(`SELECT t.*,e.host,e.port,e.sni,e.server_id,s.name server_name,av.metadata active_metadata FROM deployment_targets t LEFT JOIN certificate_versions av ON av.id=t.active_version JOIN server_endpoints e ON e.id=t.endpoint_id JOIN servers s ON s.id=e.server_id ORDER BY s.name`)
	if e != nil {
		return nil, e
	}
	var ds string
	a.Store.DB.QueryRow("SELECT value FROM settings WHERE key='expiry_days'").Scan(&ds)
	days, _ := strconv.Atoi(ds)
	serverExpiry := summarizeExpiry(targets, days, time.Now())
	byOrg, byEnv, byCert := map[string]*Node{}, map[string]*Node{}, map[string]*Node{}
	for _, o := range orgs {
		id := o["id"].(string)
		n := node("organization:"+id, id, "organization", o["name"].(string), "Unknown")
		byOrg[id] = n
		root.Children = append(root.Children, n)
	}
	for _, v := range envs {
		id := v["id"].(string)
		n := node("environment:"+id, id, "environment", v["name"].(string), "Unknown")
		if p := byOrg[v["organization_id"].(string)]; p != nil {
			p.Children = append(p.Children, n)
			byEnv[id] = n
		}
	}
	for _, v := range certs {
		id := v["id"].(string)
		status := "Pending"
		var meta map[string]any
		var val Validation
		json.Unmarshal([]byte(v["metadata"].(string)), &meta)
		json.Unmarshal([]byte(v["validation"].(string)), &val)
		if !val.Valid {
			status = "Failed"
		} else if v["approved"].(int64) == 1 {
			status = "Unknown"
		}
		if s, ok := meta["not_after"].(string); ok {
			date, _ := time.Parse(time.RFC3339, s)
			if date.Before(time.Now()) {
				status = "Failed"
			} else if time.Until(date) < time.Duration(days)*24*time.Hour {
				status = "Warning"
			}
		}
		n := node("certificate:"+id, id, "certificate", v["name"].(string), status)
		n.Data = v
		n.Data["metadata"] = meta
		n.Data["validation"] = val
		if p := byEnv[v["environment_id"].(string)]; p != nil {
			p.Children = append(p.Children, n)
			byCert[id] = n
		}
	}
	for _, v := range targets {
		id := v["id"].(string)
		n := node("target:"+id, id, "target", v["server_name"].(string)+" · "+v["sni"].(string), v["status"].(string))
		n.Data = v
		n.Data["server_expiry"] = serverExpiry[v["server_id"].(string)]
		delete(n.Data, "active_metadata")
		if p := byCert[v["certificate_id"].(string)]; p != nil {
			p.Children = append(p.Children, n)
		}
	}
	aggregate(root)
	return root, nil
}
func (a *App) tree(w http.ResponseWriter, r *http.Request) {
	tree, e := a.BuildTree()
	if e != nil {
		problem(w, 500, "tree query failed")
		return
	}
	q, status := strings.ToLower(r.URL.Query().Get("q")), r.URL.Query().Get("status")
	if q != "" || status != "" {
		var prune func(*Node) bool
		prune = func(n *Node) bool {
			children := []*Node{}
			for _, c := range n.Children {
				if prune(c) {
					children = append(children, c)
				}
			}
			self := (q == "" || strings.Contains(strings.ToLower(n.Name), q)) && (status == "" || n.Status == status)
			if !self || status != "" {
				n.Children = children
			}
			return self || len(children) > 0
		}
		prune(tree)
	}
	response(w, 200, tree)
}
func (a *App) node(w http.ResponseWriter, r *http.Request) {
	tree, e := a.BuildTree()
	if e != nil {
		problem(w, 500, "tree unavailable")
		return
	}
	var find func(*Node) *Node
	find = func(n *Node) *Node {
		if n.ID == r.PathValue("id") {
			return n
		}
		for _, c := range n.Children {
			if got := find(c); got != nil {
				return got
			}
		}
		return nil
	}
	n := find(tree)
	if n == nil {
		problem(w, 404, "node not found")
		return
	}
	response(w, 200, n)
}
func (a *App) summary(w http.ResponseWriter, r *http.Request) {
	tree, e := a.BuildTree()
	if e != nil {
		problem(w, 500, "summary unavailable")
		return
	}
	m := map[string]any{"status": tree.Status, "counts": tree.Counts}
	for key, table := range map[string]string{"certificates": "certificates", "servers": "servers", "targets": "deployment_targets", "jobs": "deployment_jobs"} {
		var count int
		a.Store.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&count)
		m[key] = count
	}
	response(w, 200, m)
}
