package app

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/certree/certree/internal/testkit"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testApp(t *testing.T, f testkit.Fixture) *App {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "ca.pem"), f.CAPEM, 0600)
	a, e := New(Config{Address: "127.0.0.1:8080", DBPath: filepath.Join(dir, "test.db"), MasterKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{4}, 32)), AdminUser: "admin", AdminPassword: "test-admin-password-2026", Origin: "http://localhost:8080", InsecureHTTP: true, CAFile: filepath.Join(dir, "ca.pem"), WebDir: dir})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { a.Store.Close() })
	return a
}

type apiClient struct {
	t       *testing.T
	a       *App
	cookies []*http.Cookie
	csrf    string
}

func (c *apiClient) request(method, path string, body any) (int, []byte) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		rd = strings.NewReader(encode(body))
	}
	r := httptest.NewRequest(method, "http://localhost:8080/api/"+path, rd)
	r.Header.Set("Origin", c.a.Config.Origin)
	r.Header.Set("X-CSRF-Token", c.csrf)
	r.Header.Set("Content-Type", "application/json")
	for _, cookie := range c.cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	c.a.Handler().ServeHTTP(w, r)
	if path == "auth/login" && w.Code == 200 {
		c.cookies = w.Result().Cookies()
		var i identity
		json.Unmarshal(w.Body.Bytes(), &i)
		c.csrf = i.CSRF
	}
	return w.Code, w.Body.Bytes()
}
func (c *apiClient) ok(method, path string, body any, status int) RowTest {
	c.t.Helper()
	code, data := c.request(method, path, body)
	if code != status {
		c.t.Fatalf("%s %s: expected %d got %d: %s", method, path, status, code, data)
	}
	var out RowTest
	json.Unmarshal(data, &out)
	return out
}

type RowTest = map[string]any

func login(t *testing.T, a *App) *apiClient {
	c := &apiClient{t: t, a: a}
	c.ok("POST", "auth/login", map[string]string{"username": "admin", "password": "test-admin-password-2026"}, 200)
	return c
}
func TestAuthenticationAndCSRF(t *testing.T) {
	a := testApp(t, fixture(t))
	c := &apiClient{t: t, a: a}
	if code, _ := c.request("GET", "tree", nil); code != 401 {
		t.Fatal("unauthenticated tree accessible")
	}
	c = login(t, a)
	if !c.cookies[0].HttpOnly || c.cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe cookie")
	}
	saved := c.csrf
	c.csrf = "invalid"
	if code, _ := c.request("POST", "organizations", map[string]string{"name": "X"}); code != 403 {
		t.Fatal("CSRF bypass")
	}
	c.csrf = saved
	c.ok("POST", "organizations", map[string]string{"name": "University"}, 201)
	r := httptest.NewRequest("POST", "http://localhost:8080/api/organizations", strings.NewReader(`{"name":"Y"}`))
	r.AddCookie(c.cookies[0])
	r.Header.Set("X-CSRF-Token", saved)
	r.Header.Set("Origin", "http://evil.invalid")
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("origin bypass")
	}
	a.Store.DB.Exec("UPDATE sessions SET expires=0")
	if code, _ := c.request("GET", "auth/me", nil); code != 401 {
		t.Fatal("expired session accepted")
	}
	for i := 0; i < 9; i++ {
		code, _ := c.request("POST", "auth/login", map[string]string{"username": "bad", "password": "wrong"})
		if i == 8 && code != 429 {
			t.Fatal("rate limit missing")
		}
	}
}
func register(t *testing.T, c *apiClient, f testkit.Fixture) (string, string, string) {
	o := c.ok("POST", "organizations", RowTest{"name": "University"}, 201)["id"].(string)
	e := c.ok("POST", "environments", RowTest{"name": "Production", "organization_id": o}, 201)["id"].(string)
	v, err := c.a.Store.Import(e, f.First, "", c.a.Roots)
	if err != nil {
		t.Fatal(err)
	}
	s := c.ok("POST", "servers", RowTest{"name": "Nginx 01", "host": "localhost", "port": 2222, "user": "certree", "type": "nginx", "host_key": string(f.HostPublic), "ssh_key": string(f.SSHKey)}, 201)["id"].(string)
	var cid string
	c.a.Store.DB.QueryRow("SELECT certificate_id FROM certificate_versions WHERE id=?", v).Scan(&cid)
	target := c.ok("POST", "targets", RowTest{"server_id": s, "certificate_id": cid, "host": "localhost", "port": 8443, "sni": "lab.test", "certificate_path": "/srv/nginx/cert.pem", "key_path": "/srv/nginx/key.pem", "configuration_path": "/srv/nginx/nginx.conf", "backup_path": "/srv/nginx/backups", "sudo": false}, 201)["id"].(string)
	return v, target, e
}
func TestAPIRelationshipsApprovalAndConcurrency(t *testing.T) {
	f := fixture(t)
	a := testApp(t, f)
	c := login(t, a)
	v, target, env := register(t, c, f)
	body := RowTest{"target_id": target, "version_id": v, "kind": "Deployment"}
	if code, _ := c.request("POST", "deployments", body); code != 422 {
		t.Fatal("unapproved version deployable")
	}
	c.ok("POST", "certificates/versions/"+v+"/approve", RowTest{}, 200)
	c.ok("POST", "deployments", body, 202)
	if code, _ := c.request("POST", "deployments", body); code != 409 {
		t.Fatal("duplicate deployment accepted")
	}
	if code, _ := c.request("POST", "deployments", RowTest{"target_id": target, "version_id": "other"}); code != 400 {
		t.Fatal("foreign version accepted")
	}
	var cid string
	a.Store.DB.QueryRow("SELECT certificate_id FROM certificate_versions WHERE id=?", v).Scan(&cid)
	tree, e := a.BuildTree()
	if e != nil {
		t.Fatal(e)
	}
	if got := tree.Children[0].Children[0].Children[0]; got.EntityID != cid || len(got.Children) != 1 {
		t.Fatal("tree relationship broken")
	}
	expired, e := a.Store.Import(env, f.Expired, "", a.Roots)
	if e != nil {
		t.Fatal(e)
	}
	if code, _ := c.request("POST", "certificates/versions/"+expired+"/approve", RowTest{}); code != 422 {
		t.Fatal("expired certificate approved")
	}
	for _, path := range []string{"certificates", "servers", "targets", "collectors"} {
		_, data := c.request("GET", path, nil)
		if bytes.Contains(data, []byte("PRIVATE KEY")) || bytes.Contains(data, f.SSHKey) {
			t.Fatal("secret leaked through " + path)
		}
	}
}
func TestMultipartImportAndInvalidInputs(t *testing.T) {
	f := fixture(t)
	a := testApp(t, f)
	c := login(t, a)
	o := c.ok("POST", "organizations", RowTest{"name": "Upload"}, 201)["id"].(string)
	env := c.ok("POST", "environments", RowTest{"name": "Prod", "organization_id": o}, 201)["id"].(string)
	var data bytes.Buffer
	m := multipart.NewWriter(&data)
	m.WriteField("environment_id", env)
	part, _ := m.CreateFormFile("files", "misleading.bin")
	part.Write(f.First)
	m.Close()
	r := httptest.NewRequest("POST", "http://localhost:8080/api/certificates", &data)
	r.Header.Set("Content-Type", m.FormDataContentType())
	r.Header.Set("Origin", a.Config.Origin)
	r.Header.Set("X-CSRF-Token", c.csrf)
	r.AddCookie(c.cookies[0])
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if code, _ := c.request("POST", "organizations", RowTest{"name": "test", "unexpected": true}); code != 400 {
		t.Fatal("unknown fields accepted")
	}
	for _, p := range []string{"relative", "/srv/../etc/key", "/", "/srv/key\n"} {
		if ValidateTarget(TargetConfig{p, "/key", "/conf", "/backup", false}) == nil {
			t.Fatal("unsafe remote path accepted")
		}
	}
	if quote("x'; touch /tmp/escape; '") != "'x'\"'\"'; touch /tmp/escape; '\"'\"''" {
		t.Fatal("shell quoting broken")
	}
}
func liveTLS(t *testing.T, f testkit.Fixture) (Target, *atomic.Pointer[tls.Certificate]) {
	first, e := tls.X509KeyPair(f.First, f.First)
	if e != nil {
		t.Fatal(e)
	}
	current := &atomic.Pointer[tls.Certificate]{}
	current.Store(&first)
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("TLS fixture")) }))
	s.TLS = &tls.Config{GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return current.Load(), nil }, MinVersion: tls.VersionTLS12}
	s.StartTLS()
	t.Cleanup(s.Close)
	port := s.Listener.Addr().(*net.TCPAddr).Port
	return Target{Host: "127.0.0.1", SNI: "lab.test", Port: port}, current
}

// workerDeployer is a test double only for job transitions; TLS verification remains real.
type workerDeployer struct {
	current            *atomic.Pointer[tls.Certificate]
	next, previous     *tls.Certificate
	old                string
	fail, recoveryFail bool
}

func (d workerDeployer) Deploy(ctx context.Context, t Target, b *Bundle, id string, stage func(string) error) error {
	if e := stage("Backed up"); e != nil {
		return e
	}
	d.current.Store(d.next)
	if d.fail {
		return errors.New("test deployment failure")
	}
	return nil
}
func (d workerDeployer) Recover(context.Context, Target, string) (string, error) {
	if d.recoveryFail {
		return "", errors.New("test recovery failure")
	}
	d.current.Store(d.previous)
	return d.old, nil
}
func TestRealTLSVerification(t *testing.T) {
	f := fixture(t)
	target, current := liveTLS(t, f)
	b, _ := ParseBundle(f.First, "")
	verifier := TLSVerifier{roots(f)}
	r := verifier.Verify(context.Background(), target, Fingerprint(b.Leaf))
	if !r.Healthy || r.Actual != Fingerprint(b.Leaf) {
		t.Fatal(r)
	}
	if verifier.Verify(context.Background(), target, "wrong").Healthy {
		t.Fatal("fingerprint mismatch accepted")
	}
	target.SNI = "unrelated.test"
	if verifier.Verify(context.Background(), target, Fingerprint(b.Leaf)).Healthy {
		t.Fatal("hostname mismatch accepted")
	}
	target.SNI = "lab.test"
	if (TLSVerifier{nil}).Verify(context.Background(), target, Fingerprint(b.Leaf)).Healthy {
		t.Fatal("untrusted CA accepted")
	}
	expired, e := tls.X509KeyPair(f.Expired, f.Expired)
	if e != nil {
		t.Fatal(e)
	}
	current.Store(&expired)
	if verifier.Verify(context.Background(), target, Fingerprint(b.Leaf)).Healthy {
		t.Fatal("expired service accepted")
	}
}
func TestWorkerSuccessRollbackAndRecoveryFailure(t *testing.T) {
	for _, scenario := range []string{"Success", "Rolled Back", "Recovery Failed", "TLS mismatch", "Interrupted"} {
		t.Run(scenario, func(t *testing.T) {
			f := fixture(t)
			a := testApp(t, f)
			c := login(t, a)
			_, target, env := register(t, c, f)
			tlsTarget, current := liveTLS(t, f)
			a.Store.DB.Exec("UPDATE server_endpoints SET host=?,port=? WHERE id=(SELECT endpoint_id FROM deployment_targets WHERE id=?)", tlsTarget.Host, tlsTarget.Port, target)
			version, e := a.Store.Import(env, f.Second, "", a.Roots)
			if e != nil {
				t.Fatal(e)
			}
			c.ok("POST", "certificates/versions/"+version+"/approve", RowTest{}, 200)
			second, e := tls.X509KeyPair(f.Second, f.Second)
			if e != nil {
				t.Fatal(e)
			}
			first := current.Load()
			b, _ := ParseBundle(f.First, "")
			d := workerDeployer{current: current, next: &second, previous: first, old: Fingerprint(b.Leaf), fail: scenario == "Rolled Back" || scenario == "Recovery Failed", recoveryFail: scenario == "Recovery Failed"}
			if scenario == "TLS mismatch" {
				d.next = first
			}
			a.Deployer = d
			id := c.ok("POST", "deployments", RowTest{"target_id": target, "version_id": version}, 202)["id"].(string)
			if scenario == "Interrupted" {
				a.Store.DB.Exec("UPDATE deployment_jobs SET status='Running' WHERE id=?", id)
				ctx, cancel := context.WithCancel(context.Background())
				done := make(chan struct{})
				go func() { a.Run(ctx); close(done) }()
				deadline := time.Now().Add(10 * time.Second)
				for {
					var status string
					a.Store.DB.QueryRow("SELECT status FROM deployment_jobs WHERE id=?", id).Scan(&status)
					if status == "Rolled Back" {
						break
					}
					if time.Now().After(deadline) {
						cancel()
						t.Fatal("interrupted job not recovered")
					}
					time.Sleep(20 * time.Millisecond)
				}
				cancel()
				<-done
			} else {
				if !a.WorkOne(context.Background()) {
					t.Fatal("job not claimed")
				}
			}
			var status, verified string
			a.Store.DB.QueryRow("SELECT status FROM deployment_jobs WHERE id=?", id).Scan(&status)
			expected := scenario
			if scenario == "TLS mismatch" || scenario == "Interrupted" {
				expected = "Rolled Back"
			}
			if status != expected {
				t.Fatal("expected", expected, "got", status)
			}
			a.Store.DB.QueryRow("SELECT verification_status FROM deployment_targets WHERE id=?", target).Scan(&verified)
			if scenario == "Recovery Failed" && verified == "Healthy" {
				t.Fatal("failed recovery marked healthy")
			}
			if expected == "Success" && verified != "Healthy" {
				t.Fatal("success without actual TLS")
			}
		})
	}
}
