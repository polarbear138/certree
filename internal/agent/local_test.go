package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/certree/certree/internal/agentwire"
	"github.com/certree/certree/internal/testkit"
)

func splitBundle(raw []byte) (cert, key []byte) {
	for {
		b, rest := pem.Decode(raw)
		if b == nil {
			break
		}
		if b.Type == "CERTIFICATE" {
			cert = append(cert, pem.EncodeToMemory(b)...)
		} else {
			key = append(key, pem.EncodeToMemory(b)...)
		}
		raw = rest
	}
	return
}
func executorFixture(t *testing.T) (Executor, agentwire.Task, []byte) {
	t.Helper()
	f, e := testkit.Generate()
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	first, key := splitBundle(f.First)
	second, nextKey := splitBundle(f.Second)
	target := agentwire.Target{ID: RandomID(), Type: "nginx", CertificatePath: filepath.Join(dir, "cert.pem"), KeyPath: filepath.Join(dir, "key.pem"), ConfigurationPath: filepath.Join(dir, "nginx.conf"), SNI: "lab.test"}
	for p, b := range map[string][]byte{target.CertificatePath: first, target.KeyPath: key, target.ConfigurationPath: []byte("test configuration")} {
		if e = os.WriteFile(p, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	fp, _ := fingerprint(second)
	x := Executor{Dir: filepath.Join(dir, "state"), Targets: []agentwire.Target{target}, Command: func(context.Context, agentwire.Target, bool) error { return nil }}
	return x, agentwire.Task{ID: RandomID(), TargetID: target.ID, Phase: "deploy", Fingerprint: fp, CertificatePEM: string(second), KeyPEM: string(nextKey)}, first
}
func TestLocalDeploymentReplayAndRollback(t *testing.T) {
	x, task, first := executorFixture(t)
	calls := 0
	x.Command = func(context.Context, agentwire.Target, bool) error { calls++; return nil }
	result, e := x.Execute(context.Background(), task)
	if e != nil || result.Outcome != "Reloaded" {
		t.Fatal(result, e)
	}
	if calls != 3 {
		t.Fatal("expected preflight, validation and reload")
	}
	_, e = x.Execute(context.Background(), task)
	if e != nil || calls != 3 {
		t.Fatal("duplicate task was executed")
	}
	task.Phase = "rollback"
	result, e = x.Execute(context.Background(), task)
	if e != nil || result.Outcome != "Rolled Back" {
		t.Fatal(result, e)
	}
	cert, _ := os.ReadFile(x.Targets[0].CertificatePath)
	if !bytes.Equal(cert, first) {
		t.Fatal("original certificate not restored")
	}
	_, e = x.Execute(context.Background(), task)
	if e != nil || calls != 5 {
		t.Fatal("rollback replay was not idempotent")
	}
}
func TestLocalFailureAndInterruptionRestoreOriginal(t *testing.T) {
	for _, scenario := range []string{"reload failure", "interrupted"} {
		t.Run(scenario, func(t *testing.T) {
			x, task, first := executorFixture(t)
			if scenario == "reload failure" {
				failed := false
				x.Command = func(_ context.Context, _ agentwire.Target, test bool) error {
					if !test && !failed {
						failed = true
						return errors.New("reload failed")
					}
					return nil
				}
			}
			result, e := x.Execute(context.Background(), task)
			if e != nil {
				t.Fatal(e)
			}
			if scenario == "interrupted" {
				p := filepath.Join(x.Dir, "jobs", task.ID, "journal.json")
				b, _ := os.ReadFile(p)
				var j journal
				json.Unmarshal(b, &j)
				j.Result = nil
				writeJSON(p, j)
				result, e = x.Execute(context.Background(), task)
			}
			if e != nil || result.Outcome != "Failed" {
				t.Fatal(result, e)
			}
			cert, _ := os.ReadFile(x.Targets[0].CertificatePath)
			if !bytes.Equal(cert, first) {
				t.Fatal("failure did not immediately restore the original certificate")
			}
		})
	}
}
func TestLocalRejectsUnconfiguredTargetAndMismatchedKey(t *testing.T) {
	x, task, first := executorFixture(t)
	bad := task
	bad.TargetID = "other"
	result, e := x.Execute(context.Background(), bad)
	if e != nil || result.Outcome != "Failed" {
		t.Fatal(result, e)
	}
	bad = task
	bad.ID = "../escape"
	if _, e = x.Execute(context.Background(), bad); e == nil {
		t.Fatal("unsafe job id accepted")
	}
	bad = task
	bad.KeyPEM = "bad key"
	result, e = x.Execute(context.Background(), bad)
	if e != nil || result.Outcome != "Failed" {
		t.Fatal(result, e)
	}
	cert, _ := os.ReadFile(x.Targets[0].CertificatePath)
	if !bytes.Equal(first, cert) {
		t.Fatal("invalid bundle modified certificate")
	}
}
func TestCLIMenuLogsAndSecretRedaction(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	if e := Save(dir, Config{Server: "https://example.test", ID: "test", Token: "must-not-appear"}); e != nil {
		t.Fatal(e)
	}
	logEvent(dir, "first")
	logEvent(dir, "second")
	for _, args := range [][]string{{"status"}, {"-logs", "--tail", "1"}, {"--logs"}, {"logs"}, {"history"}, {"help"}, {"version"}, {}} {
		var out bytes.Buffer
		args = append(args, "--state-dir", dir)
		if e := CLI(context.Background(), args, strings.NewReader("0\n"), &out); e != nil {
			t.Fatal(e)
		}
		if strings.Contains(out.String(), "must-not-appear") {
			t.Fatal("credential exposed")
		}
		if args[0] == "-logs" && strings.Contains(out.String(), "first") {
			t.Fatal("tail ignored")
		}
	}
	if _, e := NewClient(Config{Server: "http://example.test"}); e == nil {
		t.Fatal("plaintext transport permitted")
	}
}
