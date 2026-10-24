package app

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/certree/certree/internal/agent"
	"github.com/certree/certree/internal/agentwire"
)

// Real HTTPS transport, local files and live TLS verification. Only Nginx reload
// is substituted; the Linux agentintegration gate exercises the real command.
func TestAgentHTTPSDeploymentFlow(t *testing.T) {
	f := fixture(t)
	a := testApp(t, f)
	admin := login(t, a)
	_, _, env := register(t, admin, f)
	credentials := enrollTestAgent(t, a, admin, env)
	version, e := a.Store.Import(env, f.Second, "", a.Roots)
	if e != nil {
		t.Fatal(e)
	}
	admin.ok("POST", "certificates/versions/"+version+"/approve", RowTest{}, 200)
	var cid string
	a.Store.DB.QueryRow("SELECT certificate_id FROM certificate_versions WHERE id=?", version).Scan(&cid)
	live, current := liveTLS(t, f)
	spec := agentwire.Target{ID: ID(), CertificateID: cid, Type: "nginx", CertificatePath: "/etc/nginx/cert.pem", KeyPath: "/etc/nginx/key.pem", ConfigurationPath: "/etc/nginx/nginx.conf", Host: live.Host, Port: live.Port, SNI: live.SNI}
	server := httptest.NewTLSServer(a.Handler())
	defer server.Close()
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	ca := filepath.Join(dir, "server-ca.pem")
	os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	client, e := agent.NewClient(agent.Config{Server: server.URL, CAFile: ca, ID: credentials.ID, Token: credentials.Token})
	if e != nil {
		t.Fatal(e)
	}
	if e = client.Request(context.Background(), "POST", "targets", spec, nil); e != nil {
		t.Fatal(e)
	}
	local := spec
	local.CertificatePath = filepath.Join(dir, "cert.pem")
	local.KeyPath = filepath.Join(dir, "key.pem")
	local.ConfigurationPath = filepath.Join(dir, "nginx.conf")
	old, _ := ParseBundle(f.First, "")
	for p, b := range map[string][]byte{local.CertificatePath: old.PEM, local.KeyPath: old.KeyPEM, local.ConfigurationPath: []byte("fixture")} {
		if e = os.WriteFile(p, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	x := agent.Executor{Dir: filepath.Join(dir, "state"), Targets: []agentwire.Target{local}, Command: func(_ context.Context, target agentwire.Target, test bool) error {
		cert, e := os.ReadFile(target.CertificatePath)
		if e != nil {
			return e
		}
		key, e := os.ReadFile(target.KeyPath)
		if e != nil {
			return e
		}
		pair, e := tls.X509KeyPair(cert, key)
		if e == nil && !test {
			current.Store(&pair)
		}
		return e
	}}
	job := admin.ok("POST", "deployments", RowTest{"target_id": spec.ID, "version_id": version}, 202)["id"].(string)
	var reply struct {
		Task agentwire.Task `json:"task"`
	}
	if e = client.Request(context.Background(), "POST", "poll", RowTest{}, &reply); e != nil {
		t.Fatal(e)
	}
	result, e := x.Execute(context.Background(), reply.Task)
	if e != nil || result.Outcome != "Reloaded" {
		t.Fatal(result, e)
	}
	if e = client.Request(context.Background(), "POST", "jobs/"+job+"/result", result, nil); e != nil {
		t.Fatal(e)
	}
	if !a.WorkOne(context.Background()) {
		t.Fatal("verification did not run")
	}
	var status string
	a.Store.DB.QueryRow("SELECT status FROM deployment_jobs WHERE id=?", job).Scan(&status)
	if status != "Success" {
		t.Fatal("end-to-end deployment:", status)
	}
	var count int
	a.Store.DB.QueryRow("SELECT count(*) FROM verification_results WHERE job_id=?", job).Scan(&count)
	if count == 0 {
		t.Fatal("live TLS evidence missing")
	}
}
