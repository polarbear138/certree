package app

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
)

func TestManyToManyTreeAndStatusAggregation(t *testing.T) {
	f := fixture(t)
	a := testApp(t, f)
	c := login(t, a)
	v, target, env := register(t, c, f)
	c.ok("POST", "certificates/versions/"+v+"/approve", RowTest{}, 200)
	var cid, server, endpoint string
	a.Store.DB.QueryRow("SELECT certificate_id,endpoint_id FROM deployment_targets WHERE id=?", target).Scan(&cid, &endpoint)
	a.Store.DB.QueryRow("SELECT server_id FROM server_endpoints WHERE id=?", endpoint).Scan(&server)
	secondServer := c.ok("POST", "servers", RowTest{"name": "Nginx 02", "host": "localhost", "port": 2223, "user": "certree", "type": "nginx", "host_key": string(f.HostPublic), "ssh_key": string(f.SSHKey)}, 201)["id"].(string)
	otherTarget := c.ok("POST", "targets", RowTest{"server_id": secondServer, "certificate_id": cid, "host": "localhost", "port": 8444, "sni": "lab.test", "certificate_path": "/srv/nginx/cert.pem", "key_path": "/srv/nginx/key.pem", "configuration_path": "/srv/nginx/nginx.conf", "backup_path": "/srv/nginx/backups", "sudo": false}, 201)["id"].(string)
	var org string
	a.Store.DB.QueryRow("SELECT organization_id FROM environments WHERE id=?", env).Scan(&org)
	otherEnv := c.ok("POST", "environments", RowTest{"organization_id": org, "name": "Staging"}, 201)["id"].(string)
	otherVersion, e := a.Store.Import(otherEnv, f.Second, "", a.Roots)
	if e != nil {
		t.Fatal(e)
	}
	var otherCID string
	a.Store.DB.QueryRow("SELECT certificate_id FROM certificate_versions WHERE id=?", otherVersion).Scan(&otherCID)
	base := RowTest{"server_id": server, "endpoint_id": endpoint, "certificate_id": otherCID, "host": "localhost", "port": 8443, "sni": "lab.test", "certificate_path": "/srv/nginx/other-cert.pem", "key_path": "/srv/nginx/other-key.pem", "configuration_path": "/srv/nginx/nginx.conf", "backup_path": "/srv/nginx/backups", "sudo": false}
	c.ok("POST", "targets", base, 201)
	base["certificate_path"] = "/srv/nginx/cert.pem"
	if code, _ := c.request("POST", "targets", base); code != 409 {
		t.Fatal("overlapping certificate path accepted")
	}
	a.Store.DB.Exec("UPDATE deployment_targets SET status='Healthy' WHERE id=?", target)
	a.Store.DB.Exec("UPDATE deployment_targets SET status='Failed' WHERE id=?", otherTarget)
	tree, e := a.BuildTree()
	if e != nil {
		t.Fatal(e)
	}
	if tree.Status != "Failed" || tree.Counts["Healthy"] != 1 || tree.Counts["Failed"] != 1 {
		t.Fatal("incorrect aggregate", tree.Counts)
	}
	var servers, certs int
	a.Store.DB.QueryRow("SELECT count(*) FROM servers").Scan(&servers)
	a.Store.DB.QueryRow("SELECT count(*) FROM certificates").Scan(&certs)
	if servers != 2 || certs != 2 {
		t.Fatal("tree duplicated entities")
	}
	r := httptest.NewRequest("GET", "/api/tree?status=Failed", nil)
	w := httptest.NewRecorder()
	a.tree(w, r)
	if bytes.Contains(w.Body.Bytes(), []byte(`"type":"target","name":"Nginx 01`)) {
		t.Fatal("status filter retained healthy target")
	}
	firstTree, _ := a.BuildTree()
	secondTree, _ := a.BuildTree()
	if firstTree.Children[0].ID != secondTree.Children[0].ID {
		t.Fatal("unstable tree identifiers")
	}
}
func TestTreeWithOneThousandTargets(t *testing.T) {
	f := fixture(t)
	a := testApp(t, f)
	c := login(t, a)
	v, target, _ := register(t, c, f)
	c.ok("POST", "certificates/versions/"+v+"/approve", RowTest{}, 200)
	var cid, endpoint, config string
	a.Store.DB.QueryRow("SELECT certificate_id,endpoint_id,config FROM deployment_targets WHERE id=?", target).Scan(&cid, &endpoint, &config)
	var server string
	a.Store.DB.QueryRow("SELECT server_id FROM server_endpoints WHERE id=?", endpoint).Scan(&server)
	tx, e := a.Store.DB.Begin()
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 999; i++ {
		eid, tid := ID(), ID()
		if _, e = tx.Exec("INSERT INTO server_endpoints VALUES(?,?,?,?,?)", eid, server, "localhost", 8443, fmt.Sprintf("node-%d.example.test", i)); e != nil {
			t.Fatal(e)
		}
		if _, e = tx.Exec("INSERT INTO deployment_targets(id,endpoint_id,certificate_id,config,status) VALUES(?,?,?,?,?)", tid, eid, cid, config, "Healthy"); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	start := time.Now()
	tree, e := a.BuildTree()
	if e != nil {
		t.Fatal(e)
	}
	if len(tree.Children[0].Children[0].Children[0].Children) != 1000 {
		t.Fatal("large tree lost nodes")
	}
	t.Logf("1,000-target tree assembled in %s", time.Since(start))
}
func TestPasswordRotationAndCancellation(t *testing.T) {
	f := fixture(t)
	a := testApp(t, f)
	c := login(t, a)
	v, target, _ := register(t, c, f)
	c.ok("POST", "certificates/versions/"+v+"/approve", RowTest{}, 200)
	id := c.ok("POST", "deployments", RowTest{"target_id": target, "version_id": v}, 202)["id"].(string)
	c.ok("POST", "deployments/"+id+"/cancel", RowTest{}, 200)
	if a.WorkOne(t.Context()) {
		t.Fatal("cancelled job executed")
	}
	c.ok("POST", "auth/password", RowTest{"current_password": "test-admin-password-2026", "new_password": "changed-password-for-testing"}, 200)
	if code, _ := c.request("GET", "auth/me", nil); code != 401 {
		t.Fatal("old session not revoked")
	}
	if code, _ := c.request("POST", "auth/login", RowTest{"username": "admin", "password": "test-admin-password-2026"}); code != 401 {
		t.Fatal("old password accepted")
	}
	c.ok("POST", "auth/login", RowTest{"username": "admin", "password": "changed-password-for-testing"}, 200)
}
