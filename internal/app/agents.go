package app

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/certree/certree/internal/agentwire"
)

const agentSchema = `
CREATE TABLE IF NOT EXISTS agent_enrollment(token TEXT PRIMARY KEY,environment_id TEXT NOT NULL REFERENCES environments(id),expires INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS agents(id TEXT PRIMARY KEY REFERENCES servers(id),environment_id TEXT NOT NULL REFERENCES environments(id),token TEXT UNIQUE NOT NULL,revoked INTEGER NOT NULL DEFAULT 0,last_seen INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS agent_targets(target_id TEXT PRIMARY KEY REFERENCES deployment_targets(id),agent_id TEXT NOT NULL REFERENCES agents(id),spec TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS agent_tasks(job_id TEXT PRIMARY KEY REFERENCES deployment_jobs(id),phase TEXT NOT NULL DEFAULT 'deploy',report TEXT NOT NULL DEFAULT '',cause TEXT NOT NULL DEFAULT '');
INSERT OR IGNORE INTO schema_migrations VALUES(2);`

func (a *App) agentRoutes() {
	a.mux.HandleFunc("POST /api/agents/enrollment", a.protected(a.createEnrollment))
	a.mux.HandleFunc("GET /api/agents", a.protected(func(w http.ResponseWriter, r *http.Request) {
		rows, e := a.Store.Rows("SELECT a.id,s.name,a.environment_id,a.revoked,a.last_seen FROM agents a JOIN servers s ON s.id=a.id ORDER BY s.name")
		if e != nil {
			problem(w, 500, "query failed")
			return
		}
		response(w, 200, rows)
	}))
	a.mux.HandleFunc("POST /api/agents/{id}/revoke", a.protected(func(w http.ResponseWriter, r *http.Request) {
		res, e := a.Store.DB.Exec("UPDATE agents SET revoked=1 WHERE id=?", r.PathValue("id"))
		if e != nil {
			problem(w, 500, "revocation failed")
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			problem(w, 404, "agent not found")
			return
		}
		a.Store.Audit(actor(r), "agent_revoked", r.PathValue("id"))
		response(w, 200, map[string]bool{"ok": true})
	}))
	a.mux.HandleFunc("POST /api/agent/v1/register", a.registerAgent)
	a.mux.HandleFunc("GET /api/agent/v1/inventory", a.agentAuth(func(w http.ResponseWriter, r *http.Request, id string) {
		rows, e := a.Store.Rows("SELECT c.id,c.name FROM certificates c JOIN agents a ON a.environment_id=c.environment_id WHERE a.id=? ORDER BY c.name", id)
		if e != nil {
			problem(w, 500, "inventory unavailable")
			return
		}
		response(w, 200, rows)
	}))
	a.mux.HandleFunc("POST /api/agent/v1/targets", a.agentAuth(a.registerAgentTarget))
	a.mux.HandleFunc("POST /api/agent/v1/poll", a.agentAuth(a.pollAgent))
	a.mux.HandleFunc("POST /api/agent/v1/jobs/{id}/result", a.agentAuth(a.agentResult))
	a.mux.HandleFunc("GET /api/agent/v1/history", a.agentAuth(func(w http.ResponseWriter, r *http.Request, id string) {
		rows, e := a.Store.Rows("SELECT id,target_id,status,stage,error,created_at,updated_at FROM deployment_jobs WHERE server_id=? ORDER BY created_at DESC LIMIT 100", id)
		if e != nil {
			problem(w, 500, "history unavailable")
			return
		}
		response(w, 200, rows)
	}))
}

func (a *App) createEnrollment(w http.ResponseWriter, r *http.Request) {
	var b struct {
		EnvironmentID string `json:"environment_id"`
	}
	if !decode(w, r, &b) {
		return
	}
	token := ID() + ID()
	expires := time.Now().Add(15 * time.Minute).Unix()
	if _, e := a.Store.DB.Exec("INSERT INTO agent_enrollment VALUES(?,?,?)", hashToken(token), b.EnvironmentID, expires); e != nil {
		problem(w, 400, "existing environment required")
		return
	}
	a.Store.DB.Exec("DELETE FROM agent_enrollment WHERE expires<?", time.Now().Unix())
	a.Store.Audit(actor(r), "agent_enrollment_created", b.EnvironmentID)
	response(w, 201, map[string]any{"token": token, "expires": expires})
}

func (a *App) registerAgent(w http.ResponseWriter, r *http.Request) {
	var b agentwire.Registration
	if !decode(w, r, &b) {
		return
	}
	if len(b.Token) != 64 || strings.TrimSpace(b.Name) == "" || len(b.Name) > 120 {
		problem(w, 400, "registration token and name required")
		return
	}
	tx, e := a.Store.DB.Begin()
	if e != nil {
		problem(w, 500, "registration unavailable")
		return
	}
	defer tx.Rollback()
	var env string
	if e = tx.QueryRow("DELETE FROM agent_enrollment WHERE token=? AND expires>? RETURNING environment_id", hashToken(b.Token), time.Now().Unix()).Scan(&env); e != nil {
		problem(w, 401, "expired or used registration token")
		return
	}
	id, token := ID(), ID()+ID()
	if _, e = tx.Exec("INSERT INTO servers VALUES(?,?,?,?)", id, b.Name, encode(ServerConfig{Type: "agent-nginx"}), a.Store.Seal(nil)); e == nil {
		_, e = tx.Exec("INSERT INTO agents(id,environment_id,token,last_seen) VALUES(?,?,?,?)", id, env, hashToken(token), time.Now().Unix())
	}
	if e != nil {
		problem(w, 500, "registration failed")
		return
	}
	if e = tx.Commit(); e != nil {
		problem(w, 500, "registration failed")
		return
	}
	a.Store.Audit("agent:"+id, "agent_registered", id)
	response(w, 201, agentwire.Credentials{ID: id, Token: token})
}

func (a *App) agentAuth(h func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		var id string
		if len(token) != 64 || a.Store.DB.QueryRow("SELECT id FROM agents WHERE token=? AND revoked=0", hashToken(token)).Scan(&id) != nil {
			problem(w, 401, "agent authentication failed")
			return
		}
		if _, e := a.Store.DB.Exec("UPDATE agents SET last_seen=? WHERE id=?", time.Now().Unix(), id); e != nil {
			problem(w, 503, "heartbeat unavailable")
			return
		}
		h(w, r, id)
	}
}

func (a *App) registerAgentTarget(w http.ResponseWriter, r *http.Request, id string) {
	var b agentwire.Target
	if !decode(w, r, &b) {
		return
	}
	_, idErr := hex.DecodeString(b.ID)
	c := TargetConfig{CertificatePath: b.CertificatePath, KeyPath: b.KeyPath, ConfigurationPath: b.ConfigurationPath, BackupPath: "/var/lib/certree/backups"}
	if len(b.ID) != 32 || idErr != nil || b.Type != "nginx" || ValidateTarget(c) != nil || !validHost(b.Host) || !validHost(b.SNI) || strings.Contains(b.SNI, "*") || b.Port < 1 || b.Port > 65535 {
		problem(w, 400, "valid Nginx target, paths and TLS endpoint required")
		return
	}
	tx, e := a.Store.DB.Begin()
	if e != nil {
		problem(w, 500, "target registration unavailable")
		return
	}
	defer tx.Rollback()
	var allowed int
	if tx.QueryRow("SELECT count(*) FROM certificates c JOIN agents a ON c.environment_id=a.environment_id WHERE a.id=? AND c.id=?", id, b.CertificateID).Scan(&allowed) != nil || allowed != 1 {
		problem(w, 403, "certificate outside registered environment")
		return
	}
	var prior string
	if tx.QueryRow("SELECT spec FROM agent_targets WHERE target_id=? AND agent_id=?", b.ID, id).Scan(&prior) == nil {
		if prior != encode(b) {
			problem(w, 409, "registered target is immutable")
			return
		}
		response(w, 200, map[string]string{"id": b.ID})
		return
	}
	rows, e := tx.Query("SELECT spec FROM agent_targets WHERE agent_id=?", id)
	if e != nil {
		problem(w, 500, "target check failed")
		return
	}
	collision := false
	for rows.Next() {
		var raw string
		var other agentwire.Target
		if rows.Scan(&raw) != nil || json.Unmarshal([]byte(raw), &other) != nil {
			collision = true
			break
		}
		for _, p := range []string{b.CertificatePath, b.KeyPath} {
			if p == other.CertificatePath || p == other.KeyPath || p == other.ConfigurationPath {
				collision = true
			}
		}
		if b.ConfigurationPath == other.CertificatePath || b.ConfigurationPath == other.KeyPath {
			collision = true
		}
	}
	if rows.Err() != nil {
		collision = true
	}
	rows.Close()
	if collision {
		problem(w, 409, "target paths overlap")
		return
	}
	eid := ID()
	if _, e = tx.Exec("INSERT OR IGNORE INTO server_endpoints VALUES(?,?,?,?,?)", eid, id, b.Host, b.Port, b.SNI); e == nil {
		e = tx.QueryRow("SELECT id FROM server_endpoints WHERE server_id=? AND host=? AND port=? AND sni=?", id, b.Host, b.Port, b.SNI).Scan(&eid)
	}
	if e == nil {
		_, e = tx.Exec("INSERT INTO deployment_targets(id,endpoint_id,certificate_id,config) VALUES(?,?,?,?)", b.ID, eid, b.CertificateID, encode(c))
	}
	if e == nil {
		_, e = tx.Exec("INSERT INTO agent_targets VALUES(?,?,?)", b.ID, id, encode(b))
	}
	if e != nil {
		problem(w, 409, "duplicate or invalid target")
		return
	}
	if tx.Commit() != nil {
		problem(w, 500, "target registration failed")
		return
	}
	a.Store.Audit("agent:"+id, "target_created", b.ID)
	response(w, 201, map[string]string{"id": b.ID})
}

func (a *App) pollAgent(w http.ResponseWriter, r *http.Request, id string) {
	// One active job per server is enforced by the existing database index.
	var job, target, version, status string
	e := a.Store.DB.QueryRow("SELECT id,target_id,version_id,status FROM deployment_jobs WHERE server_id=? AND kind='Deployment' AND status IN ('Pending','Running') ORDER BY created_at LIMIT 1", id).Scan(&job, &target, &version, &status)
	if e == sql.ErrNoRows {
		response(w, 200, map[string]any{"task": nil})
		return
	}
	if e != nil {
		problem(w, 500, "queue unavailable")
		return
	}
	var phase string
	e = a.Store.DB.QueryRow("SELECT phase FROM agent_tasks WHERE job_id=?", job).Scan(&phase)
	if e != nil && e != sql.ErrNoRows {
		problem(w, 500, "queue unavailable")
		return
	}
	if e == sql.ErrNoRows {
		b, _, err := a.Store.Bundle(version)
		t, te := a.Store.Target(target)
		var approved int
		a.Store.DB.QueryRow("SELECT approved FROM certificate_versions WHERE id=?", version).Scan(&approved)
		if err != nil || te != nil || approved != 1 || !b.Validate(a.Roots, t.SNI).Valid {
			a.finish(job, target, "Failed", "certificate is no longer approved and valid")
			response(w, 200, map[string]any{"task": nil})
			return
		}
		tx, err := a.Store.DB.Begin()
		if err != nil {
			problem(w, 500, "queue unavailable")
			return
		}
		defer tx.Rollback()
		res, err := tx.Exec("UPDATE deployment_jobs SET status='Running',stage='Agent deploying',attempt=attempt+1,updated_at=? WHERE id=? AND status='Pending'", time.Now().UTC().Format(timestampFormat), job)
		if err != nil {
			problem(w, 500, "claim failed")
			return
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			response(w, 200, map[string]any{"task": nil})
			return
		}
		if _, err = tx.Exec("INSERT INTO agent_tasks(job_id) VALUES(?)", job); err == nil {
			_, err = tx.Exec("UPDATE deployment_targets SET status='Deploying',deployment_status='Deploying',verification_status='Pending' WHERE id=?", target)
		}
		if err != nil || tx.Commit() != nil {
			problem(w, 500, "claim failed")
			return
		}
		phase = "deploy"
	}
	if phase != "deploy" && phase != "rollback" {
		response(w, 200, map[string]any{"task": nil})
		return
	}
	task := agentwire.Task{ID: job, TargetID: target, Phase: phase}
	if phase == "deploy" {
		b, _, err := a.Store.Bundle(version)
		if err != nil {
			problem(w, 500, "bundle unavailable")
			return
		}
		task.CertificatePEM = string(b.PEM)
		task.KeyPEM = string(b.KeyPEM)
		task.Fingerprint = Fingerprint(b.Leaf)
	}
	response(w, 200, map[string]any{"task": task})
}

func (a *App) agentResult(w http.ResponseWriter, r *http.Request, id string) {
	var b agentwire.Result
	if !decode(w, r, &b) {
		return
	}
	if len(b.Error) > 2048 || (b.Phase != "deploy" && b.Phase != "rollback") || (b.Outcome != "Reloaded" && b.Outcome != "Rolled Back" && b.Outcome != "Failed" && b.Outcome != "Recovery Failed") {
		problem(w, 400, "invalid agent result")
		return
	}
	if (b.Outcome == "Reloaded" && b.Phase != "deploy") || (b.Outcome == "Rolled Back" && b.Phase != "rollback") {
		problem(w, 400, "invalid result phase")
		return
	}
	if b.Outcome == "Reloaded" || b.Outcome == "Rolled Back" {
		if raw, e := hex.DecodeString(b.Fingerprint); e != nil || len(raw) != 32 {
			problem(w, 400, "fingerprint required")
			return
		}
	}
	var phase, report string
	e := a.Store.DB.QueryRow("SELECT t.phase,t.report FROM agent_tasks t JOIN deployment_jobs j ON j.id=t.job_id WHERE j.id=? AND j.server_id=?", r.PathValue("id"), id).Scan(&phase, &report)
	if e != nil {
		problem(w, 404, "job not found")
		return
	}
	if phase != b.Phase {
		if report == encode(b) || phase == "done" || (b.Phase == "deploy" && phase == "rollback") {
			response(w, 200, map[string]bool{"ok": true})
			return
		}
		problem(w, 409, "job phase changed")
		return
	}
	res, e := a.Store.DB.Exec("UPDATE agent_tasks SET phase=?,report=? WHERE job_id=? AND phase=?", "verify-"+b.Phase, encode(b), r.PathValue("id"), b.Phase)
	if e != nil {
		problem(w, 500, "result persistence failed")
		return
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		problem(w, 409, "job phase changed")
		return
	}
	a.Store.DB.Exec("UPDATE deployment_jobs SET stage='Awaiting central TLS verification',updated_at=? WHERE id=? AND status='Running'", time.Now().UTC().Format(timestampFormat), r.PathValue("id"))
	response(w, 200, map[string]bool{"ok": true})
}

// Agent reports never establish deployment success: the server verifies live TLS.
func (a *App) workAgentVerification(ctx context.Context) bool {
	var job, target, version, phase, raw, cause string
	e := a.Store.DB.QueryRow("SELECT j.id,j.target_id,j.version_id,t.phase,t.report,t.cause FROM agent_tasks t JOIN deployment_jobs j ON j.id=t.job_id WHERE j.status='Running' AND t.phase IN ('verify-deploy','verify-rollback') ORDER BY j.created_at LIMIT 1").Scan(&job, &target, &version, &phase, &raw, &cause)
	if e != nil {
		return false
	}
	var report agentwire.Result
	json.Unmarshal([]byte(raw), &report)
	t, e := a.Store.Target(target)
	if e != nil {
		return false
	}
	finish := func(status, detail string) {
		a.finish(job, target, status, detail)
		a.Store.DB.Exec("UPDATE agent_tasks SET phase='done' WHERE job_id=? AND EXISTS(SELECT 1 FROM deployment_jobs WHERE id=? AND status NOT IN ('Pending','Running','Recovering'))", job, job)
	}
	rollback := func(detail string) {
		a.Store.DB.Exec("UPDATE agent_tasks SET phase='rollback',cause=? WHERE job_id=?", detail, job)
		a.Store.DB.Exec("UPDATE deployment_jobs SET stage='Awaiting agent rollback',error=? WHERE id=?", detail, job)
	}
	if report.Outcome == "Failed" || report.Outcome == "Recovery Failed" {
		a.Store.DB.Exec("UPDATE deployment_targets SET verification_status='Unknown' WHERE id=?", target)
		if phase == "verify-deploy" {
			rollback(report.Error)
		} else {
			finish(report.Outcome, cause+"; "+report.Error)
		}
		return true
	}
	expected := report.Fingerprint
	if phase == "verify-deploy" {
		b, _, err := a.Store.Bundle(version)
		if err != nil {
			rollback("bundle unavailable for verification")
			return true
		}
		expected = Fingerprint(b.Leaf)
		if report.Fingerprint != expected {
			rollback("agent reported a different certificate")
			return true
		}
	}
	bounded, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	v := a.verifyRetry(bounded, t, expected, job)
	if !v.Healthy {
		a.Store.DB.Exec("UPDATE deployment_targets SET verification_status='Failed' WHERE id=?", target)
		if phase == "verify-deploy" {
			rollback(v.Error)
		} else {
			finish("Recovery Failed", cause+"; restored TLS verification failed: "+v.Error)
		}
		return true
	}
	if phase == "verify-deploy" {
		finish("Success", "")
	} else {
		a.Store.DB.Exec("UPDATE deployment_targets SET verification_status='Healthy',active_version=(SELECT id FROM certificate_versions WHERE certificate_id=? AND fingerprint=? AND approved=1 LIMIT 1) WHERE id=?", t.CertificateID, expected, target)
		finish("Rolled Back", cause)
	}
	return true
}
