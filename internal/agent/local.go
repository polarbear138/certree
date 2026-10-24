package agent

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/certree/certree/internal/agentwire"
)

type Executor struct {
	Dir     string
	Targets []agentwire.Target
	Command func(context.Context, agentwire.Target, bool) error
}
type journal struct {
	TaskID          string            `json:"task_id"`
	Target          agentwire.Target  `json:"target"`
	Stage           string            `json:"stage"`
	CertificateMode os.FileMode       `json:"certificate_mode"`
	KeyMode         os.FileMode       `json:"key_mode"`
	Result          *agentwire.Result `json:"result,omitempty"`
	Rollback        *agentwire.Result `json:"rollback,omitempty"`
}

func fingerprint(p []byte) (string, error) {
	b, _ := pem.Decode(p)
	if b == nil || b.Type != "CERTIFICATE" {
		return "", errors.New("PEM certificate required")
	}
	if _, e := x509.ParseCertificate(b.Bytes); e != nil {
		return "", e
	}
	h := sha256.Sum256(b.Bytes)
	return hex.EncodeToString(h[:]), nil
}
func ValidateLocal(t agentwire.Target) error {
	if t.Type != "nginx" {
		return errors.New("only the Nginx adapter is currently supported")
	}
	if t.CertificatePath == t.KeyPath || t.CertificatePath == t.ConfigurationPath || t.KeyPath == t.ConfigurationPath {
		return errors.New("certificate, key and configuration paths must differ")
	}
	for _, p := range []string{t.CertificatePath, t.KeyPath, t.ConfigurationPath} {
		if e := safePath(p, true); e != nil {
			return e
		}
		s, e := os.Stat(p)
		if e != nil {
			return e
		}
		if !s.Mode().IsRegular() || s.Size() > 16<<20 {
			return errors.New("managed files must be regular and at most 16 MiB")
		}
		if runtime.GOOS != "windows" && s.Mode().Perm()&0022 != 0 {
			return errors.New("managed files must not be writable by group or others")
		}
	}
	st, _ := os.Stat(t.KeyPath)
	if runtime.GOOS != "windows" && st.Mode().Perm()&0077 != 0 {
		return errors.New("private key must have mode 0600 or stricter")
	}
	cert, e := os.ReadFile(t.CertificatePath)
	if e != nil {
		return e
	}
	key, e := os.ReadFile(t.KeyPath)
	if e != nil {
		return e
	}
	_, e = tls.X509KeyPair(cert, key)
	return e
}
func runCommand(ctx context.Context, t agentwire.Target, test bool) error {
	if runtime.GOOS != "linux" {
		return errors.New("local deployment is supported on Linux only")
	}
	// No shell, remote command string, or remotely supplied arguments.
	args := []string{"-c", t.ConfigurationPath, "-s", "reload"}
	if test {
		args = []string{"-t", "-c", t.ConfigurationPath}
	}
	bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(bounded, "/usr/sbin/nginx", args...)
	if e := cmd.Run(); e != nil {
		return fmt.Errorf("nginx command failed: %w (check the Nginx service log)", e)
	}
	return nil
}
func Check(ctx context.Context, t agentwire.Target) error {
	if e := ValidateLocal(t); e != nil {
		return e
	}
	return runCommand(ctx, t, true)
}

func (x Executor) Execute(ctx context.Context, task agentwire.Task) (agentwire.Result, error) {
	if raw, e := hex.DecodeString(task.ID); e != nil || len(raw) != 16 {
		return agentwire.Result{}, errors.New("invalid job id")
	}
	if task.Phase != "deploy" && task.Phase != "rollback" {
		return agentwire.Result{}, errors.New("invalid task phase")
	}
	var target *agentwire.Target
	for i := range x.Targets {
		if x.Targets[i].ID == task.TargetID {
			target = &x.Targets[i]
			break
		}
	}
	if target == nil {
		return agentwire.Result{Phase: task.Phase, Outcome: "Failed", Error: "target is not configured locally"}, nil
	}
	dir := filepath.Join(x.Dir, "jobs", task.ID)
	if e := privateDir(dir); e != nil {
		return agentwire.Result{}, e
	}
	p := filepath.Join(dir, "journal.json")
	j := journal{TaskID: task.ID, Target: *target, Stage: "preparing"}
	if e := safePath(p, false); e != nil {
		return agentwire.Result{}, e
	}
	b, e := os.ReadFile(p)
	exists := e == nil
	if exists {
		if e = json.Unmarshal(b, &j); e != nil {
			return agentwire.Result{}, e
		}
		if j.TaskID != task.ID || j.Target.ID != task.TargetID {
			return agentwire.Result{}, errors.New("journal does not match task")
		}
	} else if !os.IsNotExist(e) {
		return agentwire.Result{}, e
	}
	if task.Phase == "deploy" && j.Result != nil {
		return *j.Result, nil
	}
	if task.Phase == "rollback" && j.Rollback != nil {
		return *j.Rollback, nil
	}
	result := agentwire.Result{Phase: task.Phase}
	if task.Phase == "rollback" {
		if !exists || j.Stage == "preparing" {
			result.Outcome = "Failed"
			result.Error = "no managed files were replaced"
		} else {
			fp, err := x.restore(ctx, dir, j)
			if err != nil {
				result.Outcome = "Recovery Failed"
				result.Error = err.Error()
			} else {
				result.Outcome = "Rolled Back"
				result.Fingerprint = fp
			}
		}
		j.Rollback = &result
	} else {
		var err error
		if exists {
			err = errors.New("interrupted deployment; rollback required")
		} else {
			err = x.deploy(ctx, dir, &j, task)
		}
		if err != nil {
			result.Outcome = "Failed"
			result.Error = err.Error()
			if j.Stage != "preparing" {
				recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
				_, restoreErr := x.restore(recovery, dir, j)
				cancel()
				if restoreErr != nil {
					result.Outcome = "Recovery Failed"
					result.Error += "; local recovery: " + restoreErr.Error()
				}
			}
		} else {
			result.Outcome = "Reloaded"
			result.Fingerprint = task.Fingerprint
		}
		j.Result = &result
	}
	if len(result.Error) > 2048 {
		result.Error = result.Error[:2048]
	}
	if e = writeJSON(p, j); e != nil {
		return agentwire.Result{}, e
	}
	return result, nil
}
func (x Executor) deploy(ctx context.Context, dir string, j *journal, task agentwire.Task) error {
	t := j.Target
	if len(task.CertificatePEM)+len(task.KeyPEM) > 16<<20 {
		return errors.New("bundle too large")
	}
	if e := ValidateLocal(t); e != nil {
		return e
	}
	pair, e := tls.X509KeyPair([]byte(task.CertificatePEM), []byte(task.KeyPEM))
	if e != nil {
		return e
	}
	leaf, e := x509.ParseCertificate(pair.Certificate[0])
	if e != nil {
		return e
	}
	if e = leaf.VerifyHostname(t.SNI); e != nil {
		return e
	}
	if time.Now().Before(leaf.NotBefore) || time.Now().After(leaf.NotAfter) {
		return errors.New("certificate outside validity period")
	}
	fp, e := fingerprint([]byte(task.CertificatePEM))
	if e != nil || fp != task.Fingerprint {
		return errors.New("certificate fingerprint mismatch")
	}
	if e = x.Command(ctx, t, true); e != nil {
		return e
	}
	cert, e := os.ReadFile(t.CertificatePath)
	if e != nil {
		return e
	}
	key, e := os.ReadFile(t.KeyPath)
	if e != nil {
		return e
	}
	cs, e := os.Stat(t.CertificatePath)
	if e != nil {
		return e
	}
	ks, e := os.Stat(t.KeyPath)
	if e != nil {
		return e
	}
	j.CertificateMode = cs.Mode().Perm()
	j.KeyMode = ks.Mode().Perm()
	// Windows lacks POSIX file modes; retain safe modes in test journals.
	if runtime.GOOS == "windows" {
		j.CertificateMode = 0644
		j.KeyMode = 0600
	}
	if e = atomicWrite(filepath.Join(dir, "certificate.bak"), cert, 0600); e != nil {
		return e
	}
	if e = atomicWrite(filepath.Join(dir, "key.bak"), key, 0600); e != nil {
		return e
	}
	j.Stage = "backed-up"
	if e = writeJSON(filepath.Join(dir, "journal.json"), j); e != nil {
		return e
	}
	if e = atomicWrite(t.CertificatePath, []byte(task.CertificatePEM), j.CertificateMode); e != nil {
		return e
	}
	if e = atomicWrite(t.KeyPath, []byte(task.KeyPEM), j.KeyMode); e != nil {
		return e
	}
	if e = x.Command(ctx, t, true); e != nil {
		return e
	}
	if e = x.Command(ctx, t, false); e != nil {
		return e
	}
	j.Stage = "reloaded"
	return nil
}
func (x Executor) restore(ctx context.Context, dir string, j journal) (string, error) {
	t := j.Target
	// Validate paths, not the possibly half-replaced certificate/key pair.
	for _, p := range []string{t.CertificatePath, t.KeyPath, t.ConfigurationPath, filepath.Join(dir, "certificate.bak"), filepath.Join(dir, "key.bak")} {
		if e := safePath(p, true); e != nil {
			return "", e
		}
	}
	cert, e := os.ReadFile(filepath.Join(dir, "certificate.bak"))
	if e != nil {
		return "", e
	}
	key, e := os.ReadFile(filepath.Join(dir, "key.bak"))
	if e != nil {
		return "", e
	}
	if _, e = tls.X509KeyPair(cert, key); e != nil {
		return "", e
	}
	if j.KeyMode&0077 != 0 || j.KeyMode&0600 == 0 || j.CertificateMode&0022 != 0 {
		return "", errors.New("unsafe backup modes")
	}
	if e = atomicWrite(t.CertificatePath, cert, j.CertificateMode); e != nil {
		return "", e
	}
	if e = atomicWrite(t.KeyPath, key, j.KeyMode); e != nil {
		return "", e
	}
	if e = x.Command(ctx, t, true); e != nil {
		return "", e
	}
	if e = x.Command(ctx, t, false); e != nil {
		return "", e
	}
	return fingerprint(cert)
}

func LocalHistory(dir string) ([]map[string]any, error) {
	entries, e := os.ReadDir(filepath.Join(dir, "jobs"))
	if os.IsNotExist(e) {
		return []map[string]any{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []map[string]any{}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		b, e := os.ReadFile(filepath.Join(dir, "jobs", entry.Name(), "journal.json"))
		if e != nil {
			continue
		}
		var j journal
		if json.Unmarshal(b, &j) == nil {
			out = append(out, map[string]any{"id": j.TaskID, "target_id": j.Target.ID, "stage": j.Stage, "result": j.Result, "rollback": j.Rollback})
		}
	}
	return out, nil
}
