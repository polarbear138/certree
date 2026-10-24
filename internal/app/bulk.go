package app

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const maxBulkTargets = 200

type bulkItem struct {
	TargetID        string `json:"target_id"`
	VersionID       string `json:"version_id"`
	ServerName      string `json:"server_name"`
	CertificateName string `json:"certificate_name"`
	SNI             string `json:"sni"`
	Fingerprint     string `json:"fingerprint"`
	NotAfter        string `json:"not_after"`
	Ready           bool   `json:"ready"`
	Error           string `json:"error,omitempty"`
	JobID           string `json:"job_id,omitempty"`
}

// Resolve a concrete approved version for review, then validate that exact version
// again at submission. Never grant approval or silently swap a reviewed version.
func (a *App) prepareBulk(targetID, versionID string) bulkItem {
	item := bulkItem{TargetID: targetID, VersionID: versionID}
	t, err := a.Store.Target(targetID)
	if err != nil {
		item.Error = "Target not found"
		return item
	}
	item.SNI = t.SNI
	rows, err := a.Store.Rows(`SELECT s.name server_name,c.name certificate_name FROM server_endpoints e JOIN servers s ON s.id=e.server_id JOIN certificates c ON c.id=? WHERE e.id=?`, t.CertificateID, t.EndpointID)
	if err != nil || len(rows) != 1 {
		item.Error = "Target metadata unavailable"
		return item
	}
	item.ServerName = rows[0]["server_name"].(string)
	item.CertificateName = rows[0]["certificate_name"].(string)
	if t.Server.Type == "agent-nginx" {
		var revoked int
		if err = a.Store.DB.QueryRow("SELECT a.revoked FROM agents a JOIN agent_targets t ON t.agent_id=a.id WHERE t.target_id=?", targetID).Scan(&revoked); err != nil || revoked != 0 {
			item.Error = "Agent revoked or target not registered"
			return item
		}
	}
	if versionID == "" {
		if err = a.Store.DB.QueryRow("SELECT id FROM certificate_versions WHERE certificate_id=? AND approved=1 ORDER BY created_at DESC,id DESC LIMIT 1", t.CertificateID).Scan(&versionID); err != nil {
			item.Error = "No approved certificate version"
			return item
		}
		item.VersionID = versionID
	}
	var certificateID string
	var approved int
	if err = a.Store.DB.QueryRow("SELECT certificate_id,approved FROM certificate_versions WHERE id=?", versionID).Scan(&certificateID, &approved); err != nil || certificateID != t.CertificateID || approved != 1 {
		item.Error = "An approved version belonging to this target is required"
		return item
	}
	bundle, _, err := a.Store.Bundle(versionID)
	if err != nil {
		item.Error = "Certificate bundle unavailable"
		return item
	}
	if !bundle.Validate(a.Roots, t.SNI).Valid {
		item.Error = "Certificate is not currently trusted and valid for this endpoint"
		return item
	}
	item.Fingerprint = Fingerprint(bundle.Leaf)
	item.NotAfter = bundle.Leaf.NotAfter.UTC().Format(time.RFC3339)
	var active int
	if err = a.Store.DB.QueryRow("SELECT count(*) FROM deployment_jobs WHERE target_id=? AND status IN ('Queued','Pending','Running','Recovering')", targetID).Scan(&active); err != nil {
		item.Error = "Queue unavailable"
		return item
	}
	if active > 0 {
		item.Error = "This target already has a queued or active job"
		return item
	}
	item.Ready = true
	return item
}

func (a *App) bulkPreview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TargetIDs []string `json:"target_ids"`
	}
	if !decode(w, r, &body) {
		return
	}
	if len(body.TargetIDs) == 0 || len(body.TargetIDs) > maxBulkTargets {
		problem(w, 400, fmt.Sprintf("select between 1 and %d targets", maxBulkTargets))
		return
	}
	seen := map[string]bool{}
	items := []bulkItem{}
	for _, id := range body.TargetIDs {
		if id == "" || seen[id] {
			problem(w, 400, "target IDs must be nonempty and unique")
			return
		}
		seen[id] = true
	}
	for _, id := range body.TargetIDs {
		items = append(items, a.prepareBulk(id, ""))
	}
	response(w, 200, map[string]any{"items": items})
}

func (a *App) bulkEnqueue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []struct {
			TargetID  string `json:"target_id"`
			VersionID string `json:"version_id"`
		} `json:"items"`
	}
	if !decode(w, r, &body) {
		return
	}
	if len(body.Items) == 0 || len(body.Items) > maxBulkTargets {
		problem(w, 400, "invalid bulk target count")
		return
	}
	seen := map[string]bool{}
	for _, item := range body.Items {
		if item.TargetID == "" || item.VersionID == "" || seen[item.TargetID] {
			problem(w, 400, "unique targets and explicit version IDs required")
			return
		}
		seen[item.TargetID] = true
	}
	results := []bulkItem{}
	for _, request := range body.Items {
		item := a.prepareBulk(request.TargetID, request.VersionID)
		if item.Ready {
			id, now := ID(), time.Now().UTC().Format(timestampFormat)
			_, err := a.Store.DB.Exec(`INSERT INTO deployment_jobs(id,target_id,server_id,version_id,kind,status,created_at,updated_at) SELECT ?,t.id,e.server_id,?,'Deployment','Queued',?,? FROM deployment_targets t JOIN server_endpoints e ON e.id=t.endpoint_id WHERE t.id=?`, id, item.VersionID, now, now, item.TargetID)
			if err != nil {
				item.Ready = false
				item.Error = "Target already queued, or queue unavailable"
			} else {
				item.JobID = id
				a.Store.DB.Exec("UPDATE deployment_targets SET status='Pending' WHERE id=?", item.TargetID)
				a.Store.Audit(actor(r), "bulk_job_approved", id)
			}
		}
		results = append(results, item)
	}
	response(w, 200, map[string]any{"items": results})
}

// Only promote the oldest waiting target for each free server. The existing
// active_server_job unique index remains the final exclusion guard.
func (a *App) promoteQueued() {
	_, err := a.Store.DB.Exec(`UPDATE deployment_jobs SET status='Pending',stage='Queued for execution',updated_at=? WHERE id IN (
 SELECT j.id FROM deployment_jobs j WHERE j.status='Queued'
 AND NOT EXISTS (SELECT 1 FROM deployment_jobs a WHERE a.server_id=j.server_id AND a.status IN ('Pending','Running','Recovering'))
 AND NOT EXISTS (SELECT 1 FROM deployment_jobs q WHERE q.server_id=j.server_id AND q.status='Queued' AND (q.created_at<j.created_at OR (q.created_at=j.created_at AND q.id<j.id))))`, time.Now().UTC().Format(timestampFormat))
	if err != nil {
		slog.Warn("bulk queue promotion deferred", "error", err)
		return
	}
}
