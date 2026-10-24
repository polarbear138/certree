package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/certree/certree/internal/agentwire"
)

func agentRequest(t *testing.T, a *App, token, method, path string, body any, status int) []byte {
	t.Helper()
	r := httptest.NewRequest(method, "/api/agent/v1/"+path, strings.NewReader(encode(body)))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != status {
		t.Fatalf("%s %s: %d, want %d: %s", method, path, w.Code, status, w.Body.String())
	}
	return w.Body.Bytes()
}
func enrollTestAgent(t *testing.T, a *App, c *apiClient, env string) agentwire.Credentials {
	t.Helper()
	key := c.ok("POST", "agents/enrollment", RowTest{"environment_id": env}, 201)["token"].(string)
	var credentials agentwire.Credentials
	json.Unmarshal(agentRequest(t, a, "", "POST", "register", agentwire.Registration{Token: key, Name: "Agent 01"}, 201), &credentials)
	agentRequest(t, a, "", "POST", "register", agentwire.Registration{Token: key, Name: "Replay"}, 401)
	return credentials
}
func agentFixture(t *testing.T) (*App, *apiClient, agentwire.Credentials, string, agentwire.Target) {
	f := fixture(t)
	a := testApp(t, f)
	c := login(t, a)
	o := c.ok("POST", "organizations", RowTest{"name": "Agents"}, 201)["id"].(string)
	env := c.ok("POST", "environments", RowTest{"name": "Production", "organization_id": o}, 201)["id"].(string)
	v, e := a.Store.Import(env, f.First, "", a.Roots)
	if e != nil {
		t.Fatal(e)
	}
	var cid string
	a.Store.DB.QueryRow("SELECT certificate_id FROM certificate_versions WHERE id=?", v).Scan(&cid)
	credentials := enrollTestAgent(t, a, c, env)
	target := agentwire.Target{ID: ID(), CertificateID: cid, Type: "nginx", CertificatePath: "/etc/nginx/tls/cert.pem", KeyPath: "/etc/nginx/tls/key.pem", ConfigurationPath: "/etc/nginx/nginx.conf", Host: "localhost", Port: 443, SNI: "lab.test"}
	agentRequest(t, a, credentials.Token, "POST", "targets", target, 201)
	return a, c, credentials, v, target
}
func TestAgentEnrollmentScopeAndRevocation(t *testing.T) {
	a, c, credentials, _, target := agentFixture(t)
	agentRequest(t, a, credentials.Token, "POST", "targets", target, 200)
	changed := target
	changed.CertificatePath = "/other.pem"
	agentRequest(t, a, credentials.Token, "POST", "targets", changed, 409)
	changed = target
	changed.ID = ID()
	changed.CertificateID = "outside-environment"
	agentRequest(t, a, credentials.Token, "POST", "targets", changed, 403)
	changed = target
	changed.ID = ID()
	agentRequest(t, a, credentials.Token, "POST", "targets", changed, 409)
	agentRequest(t, a, strings.Repeat("x", 64), "GET", "inventory", nil, 401)
	data := agentRequest(t, a, credentials.Token, "GET", "inventory", nil, 200)
	if bytes.Contains(data, []byte("PRIVATE KEY")) {
		t.Fatal("inventory leaked a key")
	}
	_, data = c.request("GET", "agents", nil)
	if bytes.Contains(data, []byte(credentials.Token)) {
		t.Fatal("agent credential leaked")
	}
	var stored string
	a.Store.DB.QueryRow("SELECT token FROM agents WHERE id=?", credentials.ID).Scan(&stored)
	if stored == credentials.Token {
		t.Fatal("credential stored in plaintext")
	}
	c.ok("POST", "agents/"+credentials.ID+"/revoke", RowTest{}, 200)
	agentRequest(t, a, credentials.Token, "POST", "poll", RowTest{}, 401)
}

type agentVerifier struct {
	healthy bool
	calls   int
}

func (v *agentVerifier) Verify(_ context.Context, _ Target, expected string) Verification {
	v.calls++
	return Verification{Healthy: v.healthy, Actual: expected, Expected: expected, CheckedAt: time.Now()}
}
func TestAgentDeploymentRequiresCentralVerification(t *testing.T) {
	a, c, credentials, v, target := agentFixture(t)
	c.ok("POST", "deployments", RowTest{"target_id": target.ID, "version_id": v}, 422)
	c.ok("POST", "certificates/versions/"+v+"/approve", RowTest{}, 200)
	job := c.ok("POST", "deployments", RowTest{"target_id": target.ID, "version_id": v}, 202)["id"].(string)
	if a.WorkOne(context.Background()) {
		t.Fatal("SSH worker claimed agent deployment")
	}
	var reply struct {
		Task agentwire.Task `json:"task"`
	}
	json.Unmarshal(agentRequest(t, a, credentials.Token, "POST", "poll", RowTest{}, 200), &reply)
	if reply.Task.ID != job || reply.Task.Phase != "deploy" || reply.Task.KeyPEM == "" {
		t.Fatal("missing task")
	}
	var again struct {
		Task agentwire.Task `json:"task"`
	}
	json.Unmarshal(agentRequest(t, a, credentials.Token, "POST", "poll", RowTest{}, 200), &again)
	if again.Task.ID != job {
		t.Fatal("poll lost an unacknowledged task")
	}
	restarted, stop := context.WithCancel(context.Background())
	stop()
	a.Run(restarted)
	var restartedStatus string
	a.Store.DB.QueryRow("SELECT status FROM deployment_jobs WHERE id=?", job).Scan(&restartedStatus)
	if restartedStatus != "Running" {
		t.Fatal("server restart attempted SSH recovery for an agent", restartedStatus)
	}
	c.ok("POST", "deployments/"+job+"/cancel", RowTest{}, 409)
	result := agentwire.Result{Phase: "deploy", Outcome: "Reloaded", Fingerprint: reply.Task.Fingerprint}
	agentRequest(t, a, credentials.Token, "POST", "jobs/"+job+"/result", result, 200)
	agentRequest(t, a, credentials.Token, "POST", "jobs/"+job+"/result", result, 200)
	var status string
	a.Store.DB.QueryRow("SELECT status FROM deployment_jobs WHERE id=?", job).Scan(&status)
	if status != "Running" {
		t.Fatal("agent report marked job successful")
	}
	verifier := &agentVerifier{healthy: true}
	a.Verifier = verifier
	if !a.WorkOne(context.Background()) {
		t.Fatal("central verification not scheduled")
	}
	a.Store.DB.QueryRow("SELECT status FROM deployment_jobs WHERE id=?", job).Scan(&status)
	if status != "Success" || verifier.calls != 1 {
		t.Fatal("central verification did not finalize deployment", status)
	}
	// Completion acknowledgement is idempotent and never overwrites the verified result.
	agentRequest(t, a, credentials.Token, "POST", "jobs/"+job+"/result", result, 200)
	if a.WorkOne(context.Background()) {
		t.Fatal("completed task executed again")
	}
}
func TestAgentFailureQueuesDurableRollback(t *testing.T) {
	a, c, credentials, v, target := agentFixture(t)
	c.ok("POST", "certificates/versions/"+v+"/approve", RowTest{}, 200)
	job := c.ok("POST", "deployments", RowTest{"target_id": target.ID, "version_id": v}, 202)["id"].(string)
	agentRequest(t, a, credentials.Token, "POST", "poll", RowTest{}, 200)
	agentRequest(t, a, credentials.Token, "POST", "jobs/"+job+"/result", agentwire.Result{Phase: "deploy", Outcome: "Failed", Error: "reload failed"}, 200)
	a.WorkOne(context.Background())
	var reply struct {
		Task agentwire.Task `json:"task"`
	}
	json.Unmarshal(agentRequest(t, a, credentials.Token, "POST", "poll", RowTest{}, 200), &reply)
	if reply.Task.Phase != "rollback" || reply.Task.ID != job || reply.Task.KeyPEM != "" {
		t.Fatal("rollback command not persisted")
	}
	b, _, _ := a.Store.Bundle(v)
	a.Verifier = &agentVerifier{healthy: true}
	agentRequest(t, a, credentials.Token, "POST", "jobs/"+job+"/result", agentwire.Result{Phase: "rollback", Outcome: "Rolled Back", Fingerprint: Fingerprint(b.Leaf)}, 200)
	a.WorkOne(context.Background())
	var status string
	a.Store.DB.QueryRow("SELECT status FROM deployment_jobs WHERE id=?", job).Scan(&status)
	if status != "Rolled Back" {
		t.Fatal(status)
	}
}
func TestAgentExpiredEnrollmentAndForeignJob(t *testing.T) {
	a, c, credentials, v, target := agentFixture(t)
	var env string
	a.Store.DB.QueryRow("SELECT environment_id FROM agents WHERE id=?", credentials.ID).Scan(&env)
	key := c.ok("POST", "agents/enrollment", RowTest{"environment_id": env}, 201)["token"].(string)
	a.Store.DB.Exec("UPDATE agent_enrollment SET expires=0")
	agentRequest(t, a, "", "POST", "register", agentwire.Registration{Token: key, Name: "Expired"}, 401)
	other := enrollTestAgent(t, a, c, env)
	c.ok("POST", "certificates/versions/"+v+"/approve", RowTest{}, 200)
	job := c.ok("POST", "deployments", RowTest{"target_id": target.ID, "version_id": v}, 202)["id"].(string)
	agentRequest(t, a, credentials.Token, "POST", "poll", RowTest{}, 200)
	agentRequest(t, a, other.Token, "POST", "jobs/"+job+"/result", agentwire.Result{Phase: "deploy", Outcome: "Failed"}, 404)
	data := agentRequest(t, a, other.Token, "POST", "poll", RowTest{}, 200)
	if strings.Contains(string(data), job) {
		t.Fatal("foreign agent received task")
	}
}
