package app

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestHistoryFiltersBeforeLimit(t *testing.T) {
	f := fixture(t)
	a := testApp(t, f)
	c := login(t, a)
	version, target, _ := register(t, c, f)
	var server string
	if err := a.Store.DB.QueryRow("SELECT server_id FROM server_endpoints e JOIN deployment_targets t ON t.endpoint_id=e.id WHERE t.id=?", target).Scan(&server); err != nil {
		t.Fatal(err)
	}
	other := c.ok("POST", "servers", RowTest{"name": "Nginx 02", "host": "localhost", "port": 2223, "user": "certree", "type": "nginx", "host_key": string(f.HostPublic), "ssh_key": string(f.SSHKey)}, 201)["id"].(string)
	insert := func(id, serverID, kind, created string) {
		t.Helper()
		_, err := a.Store.DB.Exec("INSERT INTO deployment_jobs(id,target_id,server_id,version_id,kind,status,created_at,updated_at) VALUES(?,?,?,?,?,'Success',?,?)", id, target, serverID, version, kind, created, created)
		if err != nil {
			t.Fatal(err)
		}
	}
	insert("old-deployment", server, "Deployment", "2026-01-01T00:00:00Z")
	insert("old-verification", server, "Verification", "2026-01-01T00:00:00Z")
	for i := 0; i < 205; i++ {
		insert(fmt.Sprintf("noise-%03d", i), other, "Verification", "2026-10-24T00:00:00Z")
	}
	for _, kind := range []string{"Deployment", "Verification"} {
		code, data := c.request("GET", "deployments?server_id="+server+"&kind="+kind, nil)
		var rows []RowTest
		if err := json.Unmarshal(data, &rows); err != nil {
			t.Fatal(err)
		}
		if code != 200 || len(rows) != 1 || rows[0]["kind"] != kind || rows[0]["server_id"] != server || rows[0]["server_name"] != "Nginx 01" || rows[0]["sni"] != "lab.test" {
			t.Fatalf("filtered history: %d %s", code, data)
		}
	}
	code, data := c.request("GET", "deployments?kind=Verification", nil)
	var rows []RowTest
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	if code != 200 || len(rows) != 200 || rows[0]["server_id"] != other {
		t.Fatalf("history limit/order: %d %s", code, data)
	}
	if code, _ := c.request("GET", "deployments?kind=invalid", nil); code != 400 {
		t.Fatal("invalid kind accepted")
	}
	code, data = c.request("GET", "deployments?server_id=missing", nil)
	if code != 200 || string(data) != "[]\n" {
		t.Fatalf("empty server history: %d %s", code, data)
	}
}
