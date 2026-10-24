//go:build linux && agentintegration

package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// This test uses a real, isolated Nginx master and the production local adapter.
func TestRealNginxAgentDeployAndRollback(t *testing.T) {
	if _, e := os.Stat("/usr/sbin/nginx"); e != nil {
		t.Fatal("install nginx to run the agent integration gate")
	}
	x, task, original := executorFixture(t)
	x.Command = runCommand
	target := x.Targets[0]
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	dir := filepath.Dir(target.ConfigurationPath)
	config := fmt.Sprintf("pid %s/nginx.pid;\nerror_log %s/error.log;\nevents {}\nhttp { access_log off; client_body_temp_path %s/body; proxy_temp_path %s/proxy; fastcgi_temp_path %s/fastcgi; uwsgi_temp_path %s/uwsgi; scgi_temp_path %s/scgi; server { listen 127.0.0.1:%d ssl; ssl_certificate %s; ssl_certificate_key %s; location / { return 200 'ok'; } } }\n", dir, dir, dir, dir, dir, dir, dir, port, target.CertificatePath, target.KeyPath)
	if e = os.WriteFile(target.ConfigurationPath, []byte(config), 0600); e != nil {
		t.Fatal(e)
	}
	if b, e := exec.Command("/usr/sbin/nginx", "-c", target.ConfigurationPath).CombinedOutput(); e != nil {
		t.Fatalf("nginx start: %v %s", e, b)
	}
	t.Cleanup(func() {
		exec.Command("/usr/sbin/nginx", "-c", target.ConfigurationPath, "-s", "quit").Run()
		time.Sleep(100 * time.Millisecond)
	})
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(original)
	check := func(want string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			conn, e := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", fmt.Sprintf("127.0.0.1:%d", port), &tls.Config{RootCAs: roots, ServerName: "lab.test", MinVersion: tls.VersionTLS12})
			if e == nil {
				actual := conn.ConnectionState().PeerCertificates[0]
				conn.Close()
				fp, _ := fingerprint(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: actual.Raw}))
				if fp == want {
					return
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("Nginx did not serve expected certificate")
	}
	fp, _ := fingerprint(original)
	check(fp)
	result, e := x.Execute(context.Background(), task)
	if e != nil || result.Outcome != "Reloaded" {
		t.Fatal(result, e)
	}
	check(task.Fingerprint)
	task.Phase = "rollback"
	result, e = x.Execute(context.Background(), task)
	if e != nil || result.Outcome != "Rolled Back" {
		t.Fatal(result, e)
	}
	check(fp)
}
