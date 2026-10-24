package app

import (
	"encoding/json"
	"testing"
)

func TestBulkPreviewApprovalQueueAndCancellation(t *testing.T) {
	f := fixture(t)
	a := testApp(t, f)
	c := login(t, a)
	version, target, _ := register(t, c, f)
	var server, certificate string
	if err := a.Store.DB.QueryRow(`SELECT e.server_id,t.certificate_id FROM deployment_targets t JOIN server_endpoints e ON e.id=t.endpoint_id WHERE t.id=?`, target).Scan(&server, &certificate); err != nil {
		t.Fatal(err)
	}
	second := c.ok("POST", "targets", RowTest{"server_id": server, "certificate_id": certificate, "host": "localhost", "port": 8444, "sni": "lab.test", "certificate_path": "/srv/nginx/second.pem", "key_path": "/srv/nginx/second.key", "configuration_path": "/srv/nginx/nginx.conf", "backup_path": "/srv/nginx/backups"}, 201)["id"].(string)
	preview := func(ids []string) []bulkItem {
		t.Helper()
		response := c.ok("POST", "deployments/bulk/preview", RowTest{"target_ids": ids}, 200)
		raw, _ := json.Marshal(response["items"])
		var items []bulkItem
		if err := json.Unmarshal(raw, &items); err != nil {
			t.Fatal(err)
		}
		return items
	}
	if item := preview([]string{target})[0]; item.Ready || item.Error == "" {
		t.Fatal("unapproved bulk target accepted")
	}
	c.ok("POST", "certificates/versions/"+version+"/approve", RowTest{}, 200)
	items := preview([]string{target, second})
	if len(items) != 2 || !items[0].Ready || !items[1].Ready || items[0].VersionID != version || items[0].Fingerprint == "" {
		t.Fatalf("preview: %+v", items)
	}
	var count int
	a.Store.DB.QueryRow("SELECT count(*) FROM deployment_jobs").Scan(&count)
	if count != 0 {
		t.Fatal("preview created jobs")
	}
	// Explicit versions are required; submission cannot pick a different version.
	if code, _ := c.request("POST", "deployments/bulk", RowTest{"items": []RowTest{{"target_id": target}}}); code != 400 {
		t.Fatal("missing reviewed version accepted")
	}
	result := c.ok("POST", "deployments/bulk", RowTest{"items": []RowTest{{"target_id": target, "version_id": version}, {"target_id": second, "version_id": version}, {"target_id": "missing", "version_id": version}}}, 200)
	raw, _ := json.Marshal(result["items"])
	var queued []bulkItem
	json.Unmarshal(raw, &queued)
	if len(queued) != 3 || queued[0].JobID == "" || queued[1].JobID == "" || queued[2].Ready || queued[2].JobID != "" {
		t.Fatalf("bulk admission %+v", queued)
	}
	if preview([]string{target})[0].Ready {
		t.Fatal("duplicate queued target accepted")
	}
	// A single enqueue must also respect the waiting target uniqueness guard.
	if code, _ := c.request("POST", "deployments", RowTest{"target_id": target, "version_id": version}); code != 409 {
		t.Fatal("single job bypassed queued target guard")
	}
	a.promoteQueued()
	a.promoteQueued() // Repeated ticks cannot admit a second job on the same server.
	a.Store.DB.QueryRow("SELECT count(*) FROM deployment_jobs WHERE status='Pending'").Scan(&count)
	if count != 1 {
		t.Fatalf("same-server jobs not serialized: %d", count)
	}
	var pending, waiting string
	a.Store.DB.QueryRow("SELECT id FROM deployment_jobs WHERE status='Pending'").Scan(&pending)
	a.Store.DB.QueryRow("SELECT id FROM deployment_jobs WHERE status='Queued'").Scan(&waiting)
	if pending != queued[0].JobID {
		t.Fatal("waiting jobs promoted out of order")
	}
	// Waiting cancellation must not require executing any remote operation.
	c.ok("POST", "deployments/"+waiting+"/cancel", RowTest{}, 200)
	// Requeue the second target, then free the server and admit it on the next tick.
	c.ok("POST", "deployments/bulk", RowTest{"items": []RowTest{{"target_id": second, "version_id": version}}}, 200)
	c.ok("POST", "deployments/"+pending+"/cancel", RowTest{}, 200)
	a.promoteQueued()
	var next string
	a.Store.DB.QueryRow("SELECT id FROM deployment_jobs WHERE status='Pending'").Scan(&next)
	if next == "" || next == pending || next == waiting {
		t.Fatal("next target not admitted after server released")
	}
	c.ok("POST", "deployments/"+next+"/cancel", RowTest{}, 200)
	if a.WorkOne(t.Context()) {
		t.Fatal("cancelled bulk work executed")
	}
}

func TestBulkSubmissionRevalidatesReviewedApproval(t *testing.T) {
	f := fixture(t)
	a := testApp(t, f)
	c := login(t, a)
	version, target, _ := register(t, c, f)
	c.ok("POST", "certificates/versions/"+version+"/approve", RowTest{}, 200)
	if !a.prepareBulk(target, version).Ready {
		t.Fatal("fixture not ready")
	}
	if _, err := a.Store.DB.Exec("UPDATE certificate_versions SET approved=0 WHERE id=?", version); err != nil {
		t.Fatal(err)
	}
	result := c.ok("POST", "deployments/bulk", RowTest{"items": []RowTest{{"target_id": target, "version_id": version}}}, 200)
	raw, _ := json.Marshal(result["items"])
	var items []bulkItem
	json.Unmarshal(raw, &items)
	if len(items) != 1 || items[0].Ready || items[0].JobID != "" {
		t.Fatal("revoked approval submitted")
	}
	for _, ids := range [][]string{{}, {target, target}} {
		if code, _ := c.request("POST", "deployments/bulk/preview", RowTest{"target_ids": ids}); code != 400 {
			t.Fatal("invalid target set accepted")
		}
	}
}
