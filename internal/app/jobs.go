package app

import (
	"context"
	"log/slog"
	"time"
)

func (a *App) Run(ctx context.Context) {
	// Interrupted deployments are recovered, never blindly replayed.
	a.Store.DB.Exec("UPDATE deployment_jobs SET status='Recovering',stage='Interrupted; recovery required' WHERE status='Running' AND kind='Deployment'")
	a.Store.DB.Exec("UPDATE deployment_jobs SET status='Pending',stage='Interrupted; verification retry' WHERE status='Running' AND kind='Verification'")
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	poll := time.NewTicker(time.Minute)
	defer poll.Stop()
	monitor := time.NewTicker(5 * time.Minute)
	defer monitor.Stop()
	a.WorkOne(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			a.WorkOne(ctx)
		case <-poll.C:
			a.PollDue(ctx)
		case <-monitor.C:
			a.Monitor()
		}
	}
}

// Monitor only verifies known active versions; it never grants deployment approval.
func (a *App) Monitor() {
	rows, e := a.Store.Rows(`SELECT t.id,t.active_version,e.server_id FROM deployment_targets t JOIN server_endpoints e ON e.id=t.endpoint_id JOIN certificate_versions v ON v.id=t.active_version WHERE v.approved=1 ORDER BY COALESCE((SELECT MAX(created_at) FROM verification_results WHERE target_id=t.id),'')`)
	if e != nil {
		return
	}
	now := time.Now().UTC().Format(timestampFormat)
	for _, r := range rows {
		a.Store.DB.Exec("INSERT OR IGNORE INTO deployment_jobs(id,target_id,server_id,version_id,kind,status,created_at,updated_at) VALUES(?,?,?,?,'Verification','Pending',?,?)", ID(), r["id"], r["server_id"], r["active_version"], now, now)
	}
}
func (a *App) WorkOne(ctx context.Context) bool {
	var id, target, version, kind, status string
	e := a.Store.DB.QueryRow("SELECT id,target_id,version_id,kind,status FROM deployment_jobs WHERE status IN ('Pending','Recovering') AND next_run<=? ORDER BY created_at LIMIT 1", time.Now().Unix()).Scan(&id, &target, &version, &kind, &status)
	if e != nil {
		return false
	}
	res, e := a.Store.DB.Exec("UPDATE deployment_jobs SET status='Running',attempt=attempt+1,updated_at=? WHERE id=? AND status=?", time.Now().UTC().Format(timestampFormat), id, status)
	if e != nil {
		return false
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return false
	}
	jobctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	t, e := a.Store.Target(target)
	if e != nil {
		a.finish(id, target, "Failed", "target unavailable")
		return true
	}
	if status == "Recovering" {
		a.recover(jobctx, t, id, "interrupted deployment")
		return true
	}
	b, _, e := a.Store.Bundle(version)
	if e != nil {
		a.finish(id, target, "Failed", "bundle unavailable")
		return true
	}
	validation := b.Validate(a.Roots, t.SNI)
	if !validation.Valid {
		a.finish(id, target, "Failed", "certificate no longer valid")
		return true
	}
	a.Store.DB.Exec("UPDATE deployment_targets SET status='Deploying',verification_status='Pending' WHERE id=?", target)
	stage := func(s string) error {
		_, err := a.Store.DB.Exec("UPDATE deployment_jobs SET stage=?,updated_at=? WHERE id=?", s, time.Now().UTC().Format(timestampFormat), id)
		if err != nil {
			return err
		}
		_, err = a.Store.DB.Exec("INSERT INTO deployment_results VALUES(?,?,?,?,?)", ID(), id, "Running", s, time.Now().UTC().Format(timestampFormat))
		return err
	}
	if kind == "Deployment" {
		a.Store.DB.Exec("UPDATE deployment_targets SET deployment_status='Deploying' WHERE id=?", target)
		if e = stage("Deploying"); e != nil {
			a.finish(id, target, "Failed", "durable job update failed")
			return true
		}
		e = a.Deployer.Deploy(jobctx, t, b, id, stage)
		if e != nil {
			a.recover(jobctx, t, id, e.Error())
			return true
		}
		a.Store.DB.Exec("UPDATE deployment_targets SET deployment_status='Reloaded' WHERE id=?", target)
	}
	if e = stage("Verifying live TLS"); e != nil {
		if kind == "Deployment" {
			a.recover(jobctx, t, id, "durable verification update failed")
		} else {
			a.finish(id, target, "Failed", "durable verification update failed")
		}
		return true
	}
	v := a.verifyRetry(jobctx, t, Fingerprint(b.Leaf), id)
	if !v.Healthy {
		if kind == "Deployment" {
			a.recover(jobctx, t, id, v.Error)
		} else {
			var attempt int
			a.Store.DB.QueryRow("SELECT attempt FROM deployment_jobs WHERE id=?", id).Scan(&attempt)
			if attempt < 3 && ctx.Err() == nil {
				a.Store.DB.Exec("UPDATE deployment_jobs SET status='Pending',stage='Verification backoff',error=?,next_run=? WHERE id=?", v.Error, time.Now().Add(time.Duration(1<<attempt)*time.Second).Unix(), id)
				a.Store.DB.Exec("UPDATE deployment_targets SET status='Failed',verification_status='Failed' WHERE id=?", target)
			} else {
				a.finish(id, target, "Failed", v.Error)
			}
		}
		return true
	}
	a.finish(id, target, "Success", "")
	return true
}
func (a *App) verifyRetry(ctx context.Context, t Target, expected, job string) Verification {
	var v Verification
	for i := 0; i < 3; i++ {
		v = a.Verifier.Verify(ctx, t, expected)
		if _, err := a.Store.DB.Exec("INSERT INTO verification_results VALUES(?,?,?,?,?,?,?)", ID(), t.ID, job, expected, v.Actual, encode(v), time.Now().UTC().Format(timestampFormat)); err != nil {
			v.Healthy = false
			v.Error = "verification evidence could not be persisted"
			return v
		}
		if v.Healthy || ctx.Err() != nil {
			break
		}
		timer := time.NewTimer(time.Duration(1<<i) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return v
		case <-timer.C:
		}
	}
	return v
}
func (a *App) recover(ctx context.Context, t Target, job, cause string) {
	var stage string
	a.Store.DB.QueryRow("SELECT stage FROM deployment_jobs WHERE id=?", job).Scan(&stage)
	if stage == "Deploying" || stage == "Connected" {
		a.Store.DB.Exec("UPDATE deployment_targets SET verification_status='Unknown' WHERE id=?", t.ID)
		a.finish(job, t.ID, "Failed", cause+"; managed files were not replaced")
		return
	}
	a.Store.DB.Exec("UPDATE deployment_jobs SET stage='Recovering' WHERE id=?", job) // Recovery gets its own bounded context even when the deployment deadline elapsed.
	recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 90*time.Second)
	defer cancel()
	fp, e := a.Deployer.Recover(recovery, t, job)
	if e != nil {
		a.Store.DB.Exec("UPDATE deployment_targets SET verification_status='Unknown' WHERE id=?", t.ID)
		a.finish(job, t.ID, "Recovery Failed", cause+"; recovery: "+e.Error())
		return
	}
	if fp == "" {
		a.Store.DB.Exec("UPDATE deployment_targets SET verification_status='Unknown' WHERE id=?", t.ID)
		a.finish(job, t.ID, "Failed", cause+"; no files replaced")
		return
	}
	v := a.verifyRetry(recovery, t, fp, job)
	if !v.Healthy {
		a.Store.DB.Exec("UPDATE deployment_targets SET verification_status='Failed' WHERE id=?", t.ID)
		a.finish(job, t.ID, "Recovery Failed", cause+"; restored files but live TLS failed: "+v.Error)
		return
	}
	a.Store.DB.Exec("UPDATE deployment_targets SET verification_status='Healthy',active_version=(SELECT id FROM certificate_versions WHERE certificate_id=? AND fingerprint=? AND approved=1 LIMIT 1) WHERE id=?", t.CertificateID, fp, t.ID)
	a.finish(job, t.ID, "Rolled Back", cause)
}
func (a *App) finish(job, target, status, detail string) {
	now := time.Now().UTC().Format(timestampFormat)
	nodeStatus := "Failed"
	if status == "Success" {
		nodeStatus = "Healthy"
	}
	tx, err := a.Store.DB.Begin()
	if err != nil {
		slog.Error("job finalization unavailable", "id", job)
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec("UPDATE deployment_jobs SET status=?,stage=?,error=?,updated_at=? WHERE id=?", status, status, detail, now, job); err != nil {
		slog.Error("job finalization failed", "id", job)
		return
	}
	if _, err = tx.Exec(`UPDATE deployment_targets SET status=?,deployment_status=CASE WHEN (SELECT kind FROM deployment_jobs WHERE id=?)='Deployment' THEN ? ELSE deployment_status END,verification_status=CASE WHEN ?='Success' THEN 'Healthy' ELSE verification_status END,active_version=CASE WHEN ?='Success' THEN (SELECT version_id FROM deployment_jobs WHERE id=?) ELSE active_version END WHERE id=?`, nodeStatus, job, status, status, status, job, target); err != nil {
		slog.Error("target finalization failed", "id", job)
		return
	}
	if _, err = tx.Exec("INSERT INTO deployment_results VALUES(?,?,?,?,?)", ID(), job, status, detail, now); err != nil {
		slog.Error("job result persistence failed", "id", job)
		return
	}
	if _, err = tx.Exec("INSERT INTO audit_logs VALUES(?,?,?,?,?)", ID(), "worker", "job_"+status, job, now); err != nil {
		slog.Error("job audit persistence failed", "id", job)
		return
	}
	if err = tx.Commit(); err != nil {
		slog.Error("job commit failed", "id", job)
		return
	}
	slog.Info("job finished", "id", job, "status", status)
}
