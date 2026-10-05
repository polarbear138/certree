package app

import (
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"time"
)

func (a *App) password(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if !decode(w, r, &b) {
		return
	}
	if len(b.New) < 14 || len(b.New) > 72 {
		problem(w, 400, "new password must contain 14–72 bytes")
		return
	}
	var id string
	var hash []byte
	if e := a.Store.DB.QueryRow("SELECT id,password FROM users WHERE username=?", actor(r)).Scan(&id, &hash); e != nil || bcrypt.CompareHashAndPassword(hash, []byte(b.Current)) != nil {
		problem(w, 403, "current password incorrect")
		return
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(b.New), 12)
	if e != nil {
		problem(w, 500, "password update failed")
		return
	}
	tx, e := a.Store.DB.Begin()
	if e != nil {
		problem(w, 500, "transaction failed")
		return
	}
	defer tx.Rollback()
	if _, e = tx.Exec("UPDATE users SET password=? WHERE id=?", hash, id); e != nil {
		problem(w, 500, "password update failed")
		return
	}
	if _, e = tx.Exec("DELETE FROM sessions WHERE user_id=?", id); e != nil {
		problem(w, 500, "session revocation failed")
		return
	}
	if e = tx.Commit(); e != nil {
		problem(w, 500, "password update failed")
		return
	}
	a.cookie(w, "", -1)
	a.Store.Audit(actor(r), "password_changed", id)
	response(w, 200, map[string]bool{"ok": true})
}
func (a *App) cancelJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	res, e := a.Store.DB.Exec("UPDATE deployment_jobs SET status='Cancelled',stage='Cancelled by administrator',updated_at=? WHERE id=? AND status='Pending'", time.Now().UTC().Format(timestampFormat), id)
	if e != nil {
		problem(w, 500, "cancellation failed")
		return
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		problem(w, 409, "only pending jobs can be cancelled")
		return
	}
	a.Store.DB.Exec("UPDATE deployment_targets SET status='Unknown' WHERE id=(SELECT target_id FROM deployment_jobs WHERE id=?)", id)
	a.Store.Audit(actor(r), "job_cancelled", id)
	response(w, 200, map[string]bool{"ok": true})
}
