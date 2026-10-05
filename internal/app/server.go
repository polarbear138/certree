package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/subtle"
	"crypto/x509"
	"encoding/json"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Address, DBPath, MasterKey, AdminUser, AdminPassword, Origin, WebDir, CAFile string
	InsecureHTTP                                                                 bool
}
type App struct {
	Store       *Store
	Config      Config
	Roots       *x509.CertPool
	Deployer    Deployer
	Verifier    Verifier
	mux         *http.ServeMux
	rateMu      sync.Mutex
	attempts    map[string]attempt
	collectorMu sync.Mutex
}
type attempt struct {
	count int
	since time.Time
}
type identity struct{ User, CSRF string }
type identityKey struct{}

func New(c Config) (*App, error) {
	u, e := url.Parse(c.Origin)
	if e != nil || u.Host == "" || (u.Scheme != "https" && !c.InsecureHTTP) {
		return nil, errors.New("HTTPS CERTREE_ORIGIN required; HTTP is only available with explicit development mode")
	}
	if c.InsecureHTTP {
		host, _, e := net.SplitHostPort(c.Address)
		if e != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
			return nil, errors.New("development HTTP must bind loopback")
		}
	}
	s, e := Open(c.DBPath, c.MasterKey)
	if e != nil {
		return nil, e
	}
	fail := func(err error) (*App, error) { s.Close(); return nil, err }
	var count int
	s.DB.QueryRow("SELECT count(*) FROM users").Scan(&count)
	if count == 0 {
		if len(c.AdminPassword) < 14 || len(c.AdminPassword) > 72 || c.AdminUser == "" {
			return fail(errors.New("initial admin username and password of 14–72 bytes required"))
		}
		hash, e := bcrypt.GenerateFromPassword([]byte(c.AdminPassword), 12)
		if e != nil {
			return fail(e)
		}
		if _, e = s.DB.Exec("INSERT INTO users VALUES(?,?,?)", ID(), c.AdminUser, hash); e != nil {
			return fail(e)
		}
	}
	roots, e := x509.SystemCertPool()
	if e != nil {
		roots = x509.NewCertPool()
	}
	if c.CAFile != "" {
		b, e := os.ReadFile(c.CAFile)
		if e != nil {
			return fail(e)
		}
		if !roots.AppendCertsFromPEM(b) {
			return fail(errors.New("trust bundle contains no certificates"))
		}
	}
	a := &App{Store: s, Config: c, Roots: roots, Deployer: NginxDeployer{}, Verifier: TLSVerifier{roots}, mux: http.NewServeMux(), attempts: map[string]attempt{}}
	a.routes()
	return a, nil
}
func (a *App) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if !a.Config.InsecureHTTP {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		defer func() {
			if recover() != nil {
				slog.Error("request panic", "path", r.URL.Path)
				problem(w, 500, "internal error")
			}
		}()
		a.mux.ServeHTTP(w, r)
	})
}
func response(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, msg string) {
	response(w, status, map[string]string{"error": msg})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, MaxUpload)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		problem(w, 400, "invalid request fields")
		return false
	}
	if d.Decode(new(any)) != io.EOF {
		problem(w, 400, "one JSON document required")
		return false
	}
	return true
}
func (a *App) protected(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, e := r.Cookie("certree_session")
		if e != nil {
			problem(w, 401, "login required")
			return
		}
		var i identity
		var expiry int64
		e = a.Store.DB.QueryRow(`SELECT u.username,s.csrf,s.expires FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token=?`, hashToken(cookie.Value)).Scan(&i.User, &i.CSRF, &expiry)
		if e != nil || expiry < time.Now().Unix() {
			problem(w, 401, "session expired")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if r.Header.Get("Origin") != a.Config.Origin || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(i.CSRF)) != 1 {
				problem(w, 403, "CSRF validation failed")
				return
			}
		}
		h(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, i)))
	}
}
func actor(r *http.Request) string {
	i, _ := r.Context().Value(identityKey{}).(identity)
	return i.User
}
func (a *App) routes() {
	m := a.mux
	m.HandleFunc("GET /api/system/health", func(w http.ResponseWriter, r *http.Request) {
		if a.Store.DB.PingContext(r.Context()) != nil {
			problem(w, 503, "database unavailable")
			return
		}
		response(w, 200, map[string]string{"status": "ok", "version": "0.1.0"})
	})
	m.HandleFunc("POST /api/auth/login", a.login)
	m.HandleFunc("POST /api/auth/password", a.protected(a.password))
	m.HandleFunc("POST /api/deployments/{id}/cancel", a.protected(a.cancelJob))
	m.HandleFunc("GET /api/auth/me", a.protected(func(w http.ResponseWriter, r *http.Request) { response(w, 200, r.Context().Value(identityKey{})) }))
	m.HandleFunc("POST /api/auth/logout", a.protected(func(w http.ResponseWriter, r *http.Request) {
		c, _ := r.Cookie("certree_session")
		a.Store.DB.Exec("DELETE FROM sessions WHERE token=?", hashToken(c.Value))
		a.cookie(w, "", -1)
		response(w, 200, map[string]bool{"ok": true})
	}))
	for _, route := range []struct {
		pattern string
		handler http.HandlerFunc
	}{{"GET /api/tree", a.tree}, {"GET /api/tree/summary", a.summary}, {"GET /api/tree/nodes/{id}", a.node}, {"POST /api/organizations", a.organization}, {"POST /api/environments", a.environment}, {"POST /api/certificates", a.upload}, {"GET /api/certificates/{id}", a.certificate}, {"POST /api/certificates/versions/{id}/approve", a.approve}, {"POST /api/servers", a.server}, {"GET /api/servers/{id}", a.serverDetail}, {"POST /api/servers/{id}/test", a.testServer}, {"POST /api/targets", a.target}, {"POST /api/deployments", a.enqueue}, {"GET /api/deployments/{id}", a.jobDetail}, {"POST /api/collectors", a.collector}, {"POST /api/collectors/{id}/poll", a.poll}, {"POST /api/collectors/{id}/test", a.testCollector}, {"GET /api/settings", a.settings}, {"PUT /api/settings", a.settings}} {
		m.HandleFunc(route.pattern, a.protected(route.handler))
	}
	for endpoint, query := range map[string]string{"organizations": "SELECT * FROM organizations ORDER BY name", "environments": "SELECT * FROM environments ORDER BY name", "certificates": `SELECT c.*,v.id version_id,v.metadata,v.validation,v.approved,v.fingerprint,v.created_at FROM certificates c JOIN certificate_versions v ON v.id=(SELECT id FROM certificate_versions WHERE certificate_id=c.id ORDER BY created_at DESC LIMIT 1) ORDER BY c.name`, "servers": "SELECT id,name,config FROM servers ORDER BY name", "targets": "SELECT t.*,e.host,e.port,e.sni,e.server_id FROM deployment_targets t JOIN server_endpoints e ON e.id=t.endpoint_id", "deployments": "SELECT * FROM deployment_jobs ORDER BY created_at DESC LIMIT 200", "verifications": "SELECT * FROM verification_results ORDER BY created_at DESC LIMIT 200", "audit-logs": "SELECT * FROM audit_logs ORDER BY created_at DESC LIMIT 200", "collectors": "SELECT id,name,environment_id,config,last_poll,last_error FROM collectors ORDER BY name", "collector-events": "SELECT * FROM collector_events ORDER BY created_at DESC LIMIT 200"} {
		q := query
		m.HandleFunc("GET /api/"+endpoint, a.protected(func(w http.ResponseWriter, r *http.Request) {
			rows, e := a.Store.Rows(q)
			if e != nil {
				problem(w, 500, "query failed")
				return
			}
			response(w, 200, rows)
		}))
	}
	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { problem(w, 404, "API route not found") })
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			w.Header().Set("Allow", "GET, HEAD")
			problem(w, 405, "method not allowed")
			return
		}
		clean := filepath.Clean(r.URL.Path)
		f := filepath.Join(a.Config.WebDir, clean)
		if st, e := os.Stat(f); e == nil && !st.IsDir() {
			http.ServeFile(w, r, f)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(a.Config.WebDir, "index.html"))
	})
}
func (a *App) cookie(w http.ResponseWriter, token string, age int) {
	http.SetCookie(w, &http.Cookie{Name: "certree_session", Value: token, Path: "/", MaxAge: age, HttpOnly: true, Secure: !a.Config.InsecureHTTP, SameSite: http.SameSiteStrictMode})
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != a.Config.Origin {
		problem(w, 403, "origin mismatch")
		return
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	a.rateMu.Lock()
	now := time.Now()
	for k, v := range a.attempts {
		if now.Sub(v.since) > 15*time.Minute {
			delete(a.attempts, k)
		}
	}
	v := a.attempts[host]
	if v.since.IsZero() {
		v.since = now
	}
	v.count++
	a.attempts[host] = v
	blocked := v.count > 8 || len(a.attempts) > 4096
	a.rateMu.Unlock()
	if blocked {
		w.Header().Set("Retry-After", "900")
		problem(w, 429, "too many login attempts")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if !decode(w, r, &body) {
		return
	}
	if len(body.Username) == 0 || len(body.Username) > 120 || len(body.Password) > 72 {
		problem(w, 400, "invalid credential lengths")
		return
	}
	var id string
	var hash []byte
	e := a.Store.DB.QueryRow("SELECT id,password FROM users WHERE username=?", body.Username).Scan(&id, &hash)
	if e != nil {
		hash = []byte("$2a$12$T/PiPtbXDLa95uZO9DYTz.bisYrDlopREcqklLgZdPfnmcmMprAmS")
	}
	valid := bcrypt.CompareHashAndPassword(hash, []byte(body.Password)) == nil
	if e != nil || !valid {
		a.Store.Audit(body.Username, "login_failed", "")
		problem(w, 401, "invalid credentials")
		return
	}
	token, csrf := ID()+ID(), ID()+ID()
	a.Store.DB.Exec("DELETE FROM sessions WHERE expires<?", now.Unix())
	if _, e = a.Store.DB.Exec("INSERT INTO sessions VALUES(?,?,?,?)", hashToken(token), id, csrf, now.Add(8*time.Hour).Unix()); e != nil {
		problem(w, 500, "session creation failed")
		return
	}
	a.rateMu.Lock()
	delete(a.attempts, host)
	a.rateMu.Unlock()
	a.cookie(w, token, 28800)
	a.Store.Audit(body.Username, "login", id)
	response(w, 200, identity{body.Username, csrf})
}
func (a *App) organization(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &b) {
		return
	}
	if strings.TrimSpace(b.Name) == "" || len(b.Name) > 120 {
		problem(w, 400, "name required, max 120 characters")
		return
	}
	id := ID()
	if _, e := a.Store.DB.Exec("INSERT INTO organizations VALUES(?,?)", id, b.Name); e != nil {
		problem(w, 409, "organization already exists")
		return
	}
	a.Store.Audit(actor(r), "organization_created", id)
	response(w, 201, map[string]string{"id": id})
}
func (a *App) environment(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name           string `json:"name"`
		OrganizationID string `json:"organization_id"`
	}
	if !decode(w, r, &b) {
		return
	}
	if b.Name == "" || len(b.Name) > 120 {
		problem(w, 400, "name required")
		return
	}
	id := ID()
	if _, e := a.Store.DB.Exec("INSERT INTO environments VALUES(?,?,?)", id, b.OrganizationID, b.Name); e != nil {
		problem(w, 400, "invalid organization or duplicate environment")
		return
	}
	a.Store.Audit(actor(r), "environment_created", id)
	response(w, 201, map[string]string{"id": id})
}
func (a *App) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxUpload+(1<<20))
	if e := r.ParseMultipartForm(1 << 20); e != nil {
		problem(w, 400, "invalid or oversized upload")
		return
	}
	defer r.MultipartForm.RemoveAll()
	files := r.MultipartForm.File["files"]
	if len(files) == 0 || len(files) > 64 {
		problem(w, 400, "1–64 certificate bundle files required")
		return
	}
	data := []byte{}
	if len(files) == 1 {
		f, e := files[0].Open()
		if e != nil {
			problem(w, 400, "unreadable upload")
			return
		}
		data, e = io.ReadAll(io.LimitReader(f, MaxUpload+1))
		f.Close()
		if e != nil {
			problem(w, 400, "unreadable upload")
			return
		}
	} else { // Preserve file boundaries so DER and PEM can be safely mixed.
		var archive bytes.Buffer
		z := zip.NewWriter(&archive)
		for _, fh := range files {
			f, e := fh.Open()
			if e != nil {
				problem(w, 400, "unreadable file")
				return
			}
			b, e := io.ReadAll(io.LimitReader(f, MaxUpload+1))
			f.Close()
			if e != nil || len(data)+len(b) > MaxUpload {
				problem(w, 400, "upload too large")
				return
			}
			partWriter, err := z.Create(ID())
			if err != nil {
				problem(w, 500, "archive construction failed")
				return
			}
			if _, err = partWriter.Write(b); err != nil {
				problem(w, 500, "archive construction failed")
				return
			}
			data = append(data, b...)
		}
		if e := z.Close(); e != nil {
			problem(w, 500, "archive construction failed")
			return
		}
		data = archive.Bytes()
	}
	id, e := a.Store.Import(r.FormValue("environment_id"), data, r.FormValue("password"), a.Roots)
	if e != nil {
		problem(w, 400, "certificate import rejected: "+e.Error())
		return
	}
	a.Store.Audit(actor(r), "certificate_imported", id)
	response(w, 201, map[string]string{"version_id": id})
}
func (a *App) certificate(w http.ResponseWriter, r *http.Request) {
	rows, e := a.Store.Rows("SELECT id,name,environment_id FROM certificates WHERE id=?", r.PathValue("id"))
	if e != nil || len(rows) == 0 {
		problem(w, 404, "certificate not found")
		return
	}
	v, e := a.Store.Rows("SELECT id,fingerprint,metadata,validation,approved,created_at FROM certificate_versions WHERE certificate_id=? ORDER BY created_at DESC", r.PathValue("id"))
	if e != nil {
		problem(w, 500, "query failed")
		return
	}
	targets, _ := a.Store.Rows("SELECT id,status,deployment_status,verification_status FROM deployment_targets WHERE certificate_id=?", r.PathValue("id"))
	response(w, 200, map[string]any{"certificate": rows[0], "versions": v, "targets": targets})
}
func (a *App) approve(w http.ResponseWriter, r *http.Request) {
	b, _, e := a.Store.Bundle(r.PathValue("id"))
	if e != nil {
		problem(w, 404, "version not found")
		return
	}
	v := b.Validate(a.Roots, "")
	if !v.Valid {
		problem(w, 422, strings.Join(v.Errors, "; "))
		return
	}
	res, e := a.Store.DB.Exec("UPDATE certificate_versions SET approved=1,validation=? WHERE id=?", encode(v), r.PathValue("id"))
	if e != nil {
		problem(w, 500, "approval failed")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		problem(w, 404, "version not found")
		return
	}
	a.Store.Audit(actor(r), "certificate_approved", r.PathValue("id"))
	response(w, 200, v)
}
func (a *App) server(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name string `json:"name"`
		ServerConfig
		SSHKey string `json:"ssh_key"`
	}
	if !decode(w, r, &b) {
		return
	}
	if b.Name == "" || len(b.Name) > 120 || !validHost(b.Host) || b.Port < 1 || b.Port > 65535 || b.User == "" || len(b.User) > 64 || b.Type != "nginx" {
		problem(w, 400, "name, host, SSH port, user and nginx type required")
		return
	}
	if _, _, _, _, e := ssh.ParseAuthorizedKey([]byte(b.HostKey)); e != nil {
		problem(w, 400, "pinned OpenSSH host key required")
		return
	}
	if _, e := ssh.ParsePrivateKey([]byte(b.SSHKey)); e != nil {
		problem(w, 400, "unencrypted SSH private key required; stored encrypted")
		return
	}
	id := ID()
	if _, e := a.Store.DB.Exec("INSERT INTO servers VALUES(?,?,?,?)", id, b.Name, encode(b.ServerConfig), a.Store.Seal([]byte(b.SSHKey))); e != nil {
		problem(w, 500, "registration failed")
		return
	}
	a.Store.Audit(actor(r), "server_registered", id)
	response(w, 201, map[string]string{"id": id})
}
func validHost(h string) bool {
	return h != "" && len(h) <= 253 && !strings.ContainsAny(h, " /\\\r\n\t\x00")
}
func (a *App) serverDetail(w http.ResponseWriter, r *http.Request) {
	rows, e := a.Store.Rows("SELECT id,name,config FROM servers WHERE id=?", r.PathValue("id"))
	if e != nil || len(rows) == 0 {
		problem(w, 404, "server not found")
		return
	}
	response(w, 200, rows[0])
}
func (a *App) testServer(w http.ResponseWriter, r *http.Request) {
	var c string
	var enc []byte
	e := a.Store.DB.QueryRow("SELECT config,secret FROM servers WHERE id=?", r.PathValue("id")).Scan(&c, &enc)
	if e != nil {
		problem(w, 404, "server not found")
		return
	}
	t := Target{}
	json.Unmarshal([]byte(c), &t.Server)
	t.SSHKey, e = a.Store.Unseal(enc)
	if e != nil {
		problem(w, 500, "credential unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	remote, e := connect(ctx, t)
	a.Store.Audit(actor(r), "server_connection_test", r.PathValue("id"))
	if e != nil {
		problem(w, 422, "SSH connection failed: "+e.Error())
		return
	}
	remote.Close()
	response(w, 200, map[string]bool{"connected": true})
}
func (a *App) target(w http.ResponseWriter, r *http.Request) {
	var b struct {
		ServerID      string `json:"server_id"`
		CertificateID string `json:"certificate_id"`
		EndpointID    string `json:"endpoint_id"`
		Host          string `json:"host"`
		Port          int    `json:"port"`
		SNI           string `json:"sni"`
		TargetConfig
	}
	if !decode(w, r, &b) {
		return
	}
	if !validHost(b.Host) || !validHost(b.SNI) || strings.Contains(b.SNI, "*") || b.Port < 1 || b.Port > 65535 {
		problem(w, 400, "valid verification host, port and SNI required")
		return
	}
	if e := ValidateTarget(b.TargetConfig); e != nil {
		problem(w, 400, e.Error())
		return
	}
	tx, e := a.Store.DB.Begin()
	if e != nil {
		problem(w, 500, "transaction unavailable")
		return
	}
	defer tx.Rollback()
	// Distinct certificate entities must never overwrite each other's files.
	conflicts, err := tx.Query(`SELECT t.config FROM deployment_targets t JOIN server_endpoints e ON e.id=t.endpoint_id WHERE e.server_id=? AND t.certificate_id<>?`, b.ServerID, b.CertificateID)
	if err != nil {
		problem(w, 500, "target check failed")
		return
	}
	collision := false
	for conflicts.Next() {
		var raw string
		if conflicts.Scan(&raw) != nil {
			collision = true
			break
		}
		var prior TargetConfig
		json.Unmarshal([]byte(raw), &prior)
		for _, p := range []string{b.CertificatePath, b.KeyPath} {
			if p == prior.CertificatePath || p == prior.KeyPath {
				collision = true
			}
		}
	}
	conflicts.Close()
	if collision {
		problem(w, 409, "remote certificate or key path already belongs to another certificate")
		return
	}
	eid := b.EndpointID
	if eid == "" {
		eid = ID()
		if _, e = tx.Exec("INSERT OR IGNORE INTO server_endpoints VALUES(?,?,?,?,?)", eid, b.ServerID, b.Host, b.Port, b.SNI); e != nil {
			problem(w, 400, "registered server required")
			return
		}
		if e = tx.QueryRow("SELECT id FROM server_endpoints WHERE server_id=? AND host=? AND port=? AND sni=?", b.ServerID, b.Host, b.Port, b.SNI).Scan(&eid); e != nil {
			problem(w, 400, "endpoint registration failed")
			return
		}
	} else {
		var exists int
		if e = tx.QueryRow("SELECT count(*) FROM server_endpoints WHERE id=? AND server_id=? AND host=? AND port=? AND sni=?", eid, b.ServerID, b.Host, b.Port, b.SNI).Scan(&exists); e != nil || exists != 1 {
			problem(w, 400, "endpoint does not match server and TLS configuration")
			return
		}
	}
	id := ID()
	if _, e = tx.Exec("INSERT INTO deployment_targets(id,endpoint_id,certificate_id,config) VALUES(?,?,?,?)", id, eid, b.CertificateID, encode(b.TargetConfig)); e != nil {
		problem(w, 409, "invalid certificate or duplicate target")
		return
	}
	if e = tx.Commit(); e != nil {
		problem(w, 500, "registration failed")
		return
	}
	a.Store.Audit(actor(r), "target_created", id)
	response(w, 201, map[string]string{"id": id, "endpoint_id": eid})
}
func (a *App) enqueue(w http.ResponseWriter, r *http.Request) {
	var b struct {
		TargetID  string `json:"target_id"`
		VersionID string `json:"version_id"`
		Kind      string `json:"kind"`
	}
	if !decode(w, r, &b) {
		return
	}
	if b.Kind == "" {
		b.Kind = "Deployment"
	}
	if b.Kind != "Deployment" && b.Kind != "Verification" && b.Kind != "Recovery" {
		problem(w, 400, "unsupported job kind")
		return
	}
	t, e := a.Store.Target(b.TargetID)
	if e != nil {
		problem(w, 404, "target not found")
		return
	}
	var cid string
	var approved int
	e = a.Store.DB.QueryRow("SELECT certificate_id,approved FROM certificate_versions WHERE id=?", b.VersionID).Scan(&cid, &approved)
	if e != nil || cid != t.CertificateID {
		problem(w, 400, "version must belong to target certificate")
		return
	}
	bundle, _, e := a.Store.Bundle(b.VersionID)
	if e != nil {
		problem(w, 422, "bundle unavailable")
		return
	}
	v := bundle.Validate(a.Roots, t.SNI)
	if !v.Valid || approved != 1 {
		problem(w, 422, "trusted valid certificate and explicit approval required")
		return
	}
	if b.Kind == "Recovery" {
		b.Kind = "Deployment"
	}
	id := ID()
	now := time.Now().UTC().Format(timestampFormat)
	if _, e = a.Store.DB.Exec("INSERT INTO deployment_jobs(id,target_id,server_id,version_id,kind,status,created_at,updated_at) SELECT ?,?,server_id,?,?,?,?,? FROM server_endpoints WHERE id=?", id, b.TargetID, b.VersionID, b.Kind, "Pending", now, now, t.EndpointID); e != nil {
		problem(w, 409, "server already has an active job")
		return
	}
	a.Store.DB.Exec("UPDATE deployment_targets SET status='Pending' WHERE id=?", b.TargetID)
	a.Store.Audit(actor(r), "job_approved", id)
	response(w, 202, map[string]string{"id": id})
}
func (a *App) jobDetail(w http.ResponseWriter, r *http.Request) {
	rows, e := a.Store.Rows("SELECT * FROM deployment_jobs WHERE id=?", r.PathValue("id"))
	if e != nil || len(rows) == 0 {
		problem(w, 404, "job not found")
		return
	}
	results, _ := a.Store.Rows("SELECT * FROM deployment_results WHERE job_id=? ORDER BY created_at", r.PathValue("id"))
	checks, _ := a.Store.Rows("SELECT * FROM verification_results WHERE job_id=? ORDER BY created_at", r.PathValue("id"))
	response(w, 200, map[string]any{"job": rows[0], "results": results, "verifications": checks})
}
func (a *App) settings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		rows, _ := a.Store.Rows("SELECT * FROM settings")
		response(w, 200, rows)
		return
	}
	var b struct {
		ExpiryDays int `json:"expiry_days"`
	}
	if !decode(w, r, &b) {
		return
	}
	if b.ExpiryDays < 1 || b.ExpiryDays > 365 {
		problem(w, 400, "expiry_days must be 1–365")
		return
	}
	a.Store.DB.Exec("UPDATE settings SET value=? WHERE key='expiry_days'", strconv.Itoa(b.ExpiryDays))
	a.Store.Audit(actor(r), "settings_updated", "expiry_days")
	response(w, 200, map[string]bool{"ok": true})
}
