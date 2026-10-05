//go:build integration

package app

import (
	"context"
	"encoding/pem"
	"errors"
	"github.com/certree/certree/internal/testkit"
	"net/smtp"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func labEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func labPort(k, d string) int {
	n, e := strconv.Atoi(labEnv(k, d))
	if e != nil {
		panic(e)
	}
	return n
}
func TestNginxLabLifecycle(t *testing.T) {
	dir := os.Getenv("CERTREE_LAB_DIR")
	if dir == "" {
		t.Fatal("integration tag requires CERTREE_LAB_DIR and a running compose.lab.yaml environment")
	}
	read := func(name string) []byte {
		b, e := os.ReadFile(filepath.Join(dir, name))
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	f := testkit.Fixture{CAPEM: read("ca.pem"), First: read("v1.pem"), Second: read("v2.pem"), Expired: read("expired.pem"), SSHKey: read("ssh.key"), HostPublic: read("host.pub")}
	a := testApp(t, f)
	c := login(t, a)
	o := c.ok("POST", "organizations", RowTest{"name": "Integration"}, 201)["id"].(string)
	env := c.ok("POST", "environments", RowTest{"organization_id": o, "name": "Production"}, 201)["id"].(string)
	collector := c.ok("POST", "collectors", RowTest{"name": "Lab mail", "environment_id": env, "host": labEnv("CERTREE_LAB_IMAP_HOST", "localhost"), "port": labPort("CERTREE_LAB_IMAP_PORT", "1993"), "username": "username", "password": "password", "mailbox": "INBOX", "from": "supplier@example.test", "subject": "TLS renewal", "interval": 60}, 201)["id"].(string)
	var sendErr error
	for i := 0; i < 30; i++ {
		sendErr = smtp.SendMail(labEnv("CERTREE_LAB_SMTP", "localhost:1025"), nil, "supplier@example.test", []string{"certree@example.test"}, emailAttachment(f.Second))
		if sendErr == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if sendErr != nil {
		t.Fatal(sendErr)
	}
	c.ok("POST", "collectors/"+collector+"/test", RowTest{}, 200)
	c.ok("POST", "collectors/"+collector+"/poll", RowTest{}, 202)
	a.PollDue(context.Background())
	var version, cid string
	if e := a.Store.DB.QueryRow("SELECT id,certificate_id FROM certificate_versions").Scan(&version, &cid); e != nil {
		t.Fatal("mail certificate not imported", e)
	}
	server := c.ok("POST", "servers", RowTest{"name": "Nginx lab", "host": labEnv("CERTREE_LAB_HOST", "localhost"), "port": labPort("CERTREE_LAB_SSH_PORT", "12222"), "user": "certree", "type": "nginx", "host_key": string(f.HostPublic), "ssh_key": string(f.SSHKey)}, 201)["id"].(string)
	c.ok("POST", "servers/"+server+"/test", RowTest{}, 200)
	target := c.ok("POST", "targets", RowTest{"server_id": server, "certificate_id": cid, "host": labEnv("CERTREE_LAB_HOST", "localhost"), "port": labPort("CERTREE_LAB_TLS_PORT", "18443"), "sni": "lab.test", "certificate_path": "/srv/nginx/cert.pem", "key_path": "/srv/nginx/key.pem", "configuration_path": "/srv/nginx/nginx.conf", "backup_path": "/srv/nginx/backups", "sudo": false}, 201)["id"].(string)
	t.Run("unapproved_email_is_blocked", func(t *testing.T) {
		if status, _ := c.request("POST", "deployments", RowTest{"target_id": target, "version_id": version}); status != 422 {
			t.Fatal("email deployment approved without admin")
		}
	})
	c.ok("POST", "certificates/versions/"+version+"/approve", RowTest{}, 200)
	enqueue := func(v string) string {
		return c.ok("POST", "deployments", RowTest{"target_id": target, "version_id": v}, 202)["id"].(string)
	}
	outcome := func(id, expected string) {
		t.Helper()
		if !a.WorkOne(context.Background()) {
			t.Fatal("job not claimed")
		}
		var status, detail string
		a.Store.DB.QueryRow("SELECT status,error FROM deployment_jobs WHERE id=?", id).Scan(&status, &detail)
		if status != expected {
			t.Fatalf("expected %s got %s: %s", expected, status, detail)
		}
	}
	t.Run("real_nginx_deploy_and_verify", func(t *testing.T) {
		id := enqueue(version)
		if status, _ := c.request("POST", "deployments", RowTest{"target_id": target, "version_id": version}); status != 409 {
			t.Fatal("duplicate not blocked")
		}
		outcome(id, "Success")
		tree, e := a.BuildTree()
		if e != nil || tree.Status != "Healthy" {
			t.Fatal("tree not healthy", e)
		}
		targetData, e := a.Store.Target(target)
		if e != nil {
			t.Fatal(e)
		}
		r, e := connect(context.Background(), targetData)
		if e != nil {
			t.Fatal(e)
		}
		defer r.Close()
		st, e := r.files.Stat(targetData.Config.KeyPath)
		if e != nil || st.Mode().Perm() != 0600 {
			t.Fatal("private key mode is not 0600", e)
		}
		b, _, _ := a.Store.Bundle(version)
		actual, _ := r.read(targetData.Config.CertificatePath)
		if !strings.Contains(string(actual), string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: b.Leaf.Raw}))) {
			t.Fatal("deployed leaf missing")
		}
	})
	old, e := a.Store.Import(env, f.First, "", a.Roots)
	if e != nil {
		t.Fatal(e)
	}
	c.ok("POST", "certificates/versions/"+old+"/approve", RowTest{}, 200)
	t.Run("manual_restore_of_previous_version", func(t *testing.T) { outcome(enqueue(old), "Success") })
	t.Run("nginx_configuration_failure_rolls_back", func(t *testing.T) {
		id := enqueue(version)
		targetData, _ := a.Store.Target(target)
		actual := NginxDeployer{}
		a.Deployer = &configurationFailure{actual: actual}
		outcome(id, "Rolled Back")
		a.Deployer = actual
		verifier := a.Verifier.Verify(context.Background(), targetData, mustFingerprint(t, f.First))
		if !verifier.Healthy {
			t.Fatal(verifier)
		}
	})
	t.Run("live_tls_mismatch_rolls_back", func(t *testing.T) {
		a.Store.DB.Exec("UPDATE server_endpoints SET sni='wrong.test' WHERE id=(SELECT endpoint_id FROM deployment_targets WHERE id=?)", target)
		if status, _ := c.request("POST", "deployments", RowTest{"target_id": target, "version_id": version}); status != 422 {
			t.Fatal("domain validation bypassed")
		}
		a.Store.DB.Exec("UPDATE server_endpoints SET sni='lab.test' WHERE id=(SELECT endpoint_id FROM deployment_targets WHERE id=?)", target) // Inject a verifier expectation mismatch while the actual nginx transport stays real.
		id := enqueue(version)
		realVerifier := a.Verifier
		alternate, _ := liveTLS(t, f)
		a.Verifier = mismatchOnce{real: realVerifier, alternate: alternate, reject: mustFingerprint(t, f.Second)}
		outcome(id, "Rolled Back")
		a.Verifier = realVerifier
	})
	t.Run("expired_import_cannot_be_approved", func(t *testing.T) {
		expired, e := a.Store.Import(env, f.Expired, "", a.Roots)
		if e != nil {
			t.Fatal(e)
		}
		if status, _ := c.request("POST", "certificates/versions/"+expired+"/approve", RowTest{}); status != 422 {
			t.Fatal("expired approval accepted")
		}
	})
	t.Run("restart_recovers_interrupted_file_replacement", func(t *testing.T) {
		id := enqueue(version)
		a.Deployer = interruptedNginx{NginxDeployer{}}
		crashed := false
		func() {
			defer func() {
				if recover() != nil {
					crashed = true
				}
			}()
			a.WorkOne(context.Background())
		}()
		if !crashed {
			t.Fatal("crash injection was not reached")
		}
		cfg := a.Config
		a.Store.Close()
		restarted, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		a = restarted
		c.a = restarted
		t.Cleanup(func() { restarted.Store.Close() })
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { a.Run(ctx); close(done) }()
		deadline := time.Now().Add(20 * time.Second)
		for {
			var status string
			a.Store.DB.QueryRow("SELECT status FROM deployment_jobs WHERE id=?", id).Scan(&status)
			if status == "Rolled Back" {
				break
			}
			if status == "Recovery Failed" || time.Now().After(deadline) {
				cancel()
				<-done
				t.Fatal("restart recovery failed", status)
			}
			time.Sleep(50 * time.Millisecond)
		}
		cancel()
		<-done
	})
	t.Run("missing_backup_records_recovery_failed", func(t *testing.T) {
		a.Deployer = missingBackupNginx{NginxDeployer{}}
		outcome(enqueue(version), "Recovery Failed")
		a.Deployer = NginxDeployer{}
		outcome(enqueue(old), "Success")
	})
	t.Run("initial_ssh_failure_does_not_claim_rollback", func(t *testing.T) {
		var raw string
		a.Store.DB.QueryRow("SELECT config FROM servers WHERE id=?", server).Scan(&raw)
		bad := ServerConfig{Host: "127.0.0.1", Port: 1, User: "certree", HostKey: string(f.HostPublic), Type: "nginx"}
		a.Store.DB.Exec("UPDATE servers SET config=? WHERE id=?", encode(bad), server)
		outcome(enqueue(old), "Failed")
		a.Store.DB.Exec("UPDATE servers SET config=? WHERE id=?", raw, server)
	})
}

type interruptedNginx struct{ NginxDeployer }

func (d interruptedNginx) Deploy(ctx context.Context, t Target, b *Bundle, id string, stage func(string) error) error {
	return d.NginxDeployer.Deploy(ctx, t, b, id, func(s string) error {
		if e := stage(s); e != nil {
			return e
		}
		if s == "Certificate replaced" {
			panic("isolated lab interruption")
		}
		return nil
	})
}

type missingBackupNginx struct{ NginxDeployer }

func (d missingBackupNginx) Deploy(ctx context.Context, t Target, b *Bundle, id string, stage func(string) error) error {
	return d.NginxDeployer.Deploy(ctx, t, b, id, func(s string) error {
		if e := stage(s); e != nil {
			return e
		}
		if s == "Certificate replaced" {
			r, e := connect(ctx, t)
			if e != nil {
				return e
			}
			defer r.Close()
			if e = r.files.Remove(jobDir(t, id) + "/key"); e != nil {
				return e
			}
			return errors.New("isolated lab backup removed")
		}
		return nil
	})
}
func mustFingerprint(t *testing.T, b []byte) string {
	leaf, e := ParseBundle(b, "")
	if e != nil {
		t.Fatal(e)
	}
	return Fingerprint(leaf.Leaf)
}

type configurationFailure struct{ actual NginxDeployer }

func (d *configurationFailure) Deploy(ctx context.Context, t Target, b *Bundle, id string, stage func(string) error) error {
	return d.actual.Deploy(ctx, t, b, id, func(s string) error {
		if s == "Certificate replaced" {
			r, e := connect(ctx, t)
			if e != nil {
				return e
			}
			defer r.Close()
			if e = r.replace(t.Config.ConfigurationPath, t.Config.ConfigurationPath+".test-invalid", []byte("this is deliberately invalid nginx syntax;"), 0600); e != nil {
				return e
			}
		}
		return stage(s)
	})
}
func (d *configurationFailure) Recover(ctx context.Context, t Target, id string) (string, error) {
	return d.actual.Recover(ctx, t, id)
}

type mismatchOnce struct {
	real      Verifier
	alternate Target
	reject    string
}

func (v mismatchOnce) Verify(ctx context.Context, t Target, expected string) Verification {
	if expected == v.reject {
		return v.real.Verify(ctx, v.alternate, expected)
	}
	return v.real.Verify(ctx, t, expected)
}
