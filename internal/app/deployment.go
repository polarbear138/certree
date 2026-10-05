package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"time"
)

type ServerConfig struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	User    string `json:"user"`
	HostKey string `json:"host_key"`
	Type    string `json:"type"`
}
type TargetConfig struct {
	CertificatePath   string `json:"certificate_path"`
	KeyPath           string `json:"key_path"`
	ConfigurationPath string `json:"configuration_path"`
	BackupPath        string `json:"backup_path"`
	Sudo              bool   `json:"sudo"`
}
type Target struct {
	ID, CertificateID, EndpointID, Host, SNI string
	Port                                     int
	Server                                   ServerConfig
	Config                                   TargetConfig
	SSHKey                                   []byte
}
type Verification struct {
	Healthy    bool      `json:"healthy"`
	Expected   string    `json:"expected"`
	Actual     string    `json:"actual"`
	Error      string    `json:"error,omitempty"`
	SAN        []string  `json:"san,omitempty"`
	NotAfter   time.Time `json:"not_after"`
	TLSVersion uint16    `json:"tls_version"`
	CheckedAt  time.Time `json:"checked_at"`
}
type Verifier interface {
	Verify(context.Context, Target, string) Verification
}
type TLSVerifier struct{ Roots *x509.CertPool }

func (v TLSVerifier) Verify(ctx context.Context, t Target, expected string) Verification {
	r := Verification{Expected: expected, CheckedAt: time.Now().UTC()}
	raw, e := (&net.Dialer{Timeout: 8 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(t.Host, fmt.Sprint(t.Port)))
	if e != nil {
		r.Error = e.Error()
		return r
	}
	defer raw.Close()
	raw.SetDeadline(time.Now().Add(8 * time.Second))
	conn := tls.Client(raw, &tls.Config{ServerName: t.SNI, RootCAs: v.Roots, MinVersion: tls.VersionTLS12})
	e = conn.HandshakeContext(ctx)
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		r.Error = "peer did not present a certificate"
		if e != nil {
			r.Error = e.Error()
		}
		return r
	}
	c := state.PeerCertificates[0]
	r.Actual = Fingerprint(c)
	r.SAN = c.DNSNames
	r.NotAfter = c.NotAfter
	r.TLSVersion = state.Version
	if e != nil {
		r.Error = e.Error()
		return r
	}
	r.Healthy = subtle.ConstantTimeCompare([]byte(expected), []byte(r.Actual)) == 1
	if !r.Healthy {
		r.Error = "served certificate fingerprint does not match approved version"
	}
	return r
}

type Deployer interface {
	Deploy(context.Context, Target, *Bundle, string, func(string) error) error
	Recover(context.Context, Target, string) (string, error)
}
type NginxDeployer struct{}
type remote struct {
	ssh   *ssh.Client
	files *sftp.Client
	conn  net.Conn
	stop  func() bool
}

func connect(ctx context.Context, t Target) (*remote, error) {
	signer, e := ssh.ParsePrivateKey(t.SSHKey)
	if e != nil {
		return nil, errors.New("invalid SSH private key")
	}
	pub, _, _, _, e := ssh.ParseAuthorizedKey([]byte(t.Server.HostKey))
	if e != nil {
		return nil, errors.New("host_key must contain a pinned OpenSSH public key")
	}
	cfg := &ssh.ClientConfig{User: t.Server.User, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, HostKeyCallback: ssh.FixedHostKey(pub), Timeout: 10 * time.Second}
	c, e := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(t.Server.Host, fmt.Sprint(t.Server.Port)))
	if e != nil {
		return nil, e
	}
	c.SetDeadline(time.Now().Add(60 * time.Second))
	stop := context.AfterFunc(ctx, func() { c.Close() })
	cc, ch, req, e := ssh.NewClientConn(c, net.JoinHostPort(t.Server.Host, fmt.Sprint(t.Server.Port)), cfg)
	if e != nil {
		stop()
		c.Close()
		return nil, e
	}
	client := ssh.NewClient(cc, ch, req)
	files, e := sftp.NewClient(client)
	if e != nil {
		stop()
		client.Close()
		return nil, e
	}
	return &remote{client, files, c, stop}, nil
}
func (r *remote) Close()    { r.stop(); r.files.Close(); r.ssh.Close() }
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func validPath(p string) bool {
	return strings.HasPrefix(p, "/") && path.Clean(p) == p && p != "/" && !strings.ContainsAny(p, "\x00\r\n")
}
func ValidateTarget(c TargetConfig) error {
	for _, p := range []string{c.CertificatePath, c.KeyPath, c.ConfigurationPath, c.BackupPath} {
		if !validPath(p) {
			return errors.New("remote paths must be canonical absolute POSIX paths")
		}
	}
	ps := []string{c.CertificatePath, c.KeyPath, c.ConfigurationPath}
	for i, p := range ps {
		if p == c.BackupPath || strings.HasPrefix(p, c.BackupPath+"/") {
			return errors.New("backup directory overlaps managed files")
		}
		for j, q := range ps {
			if i != j && (p == q || strings.HasPrefix(p, q+"/")) {
				return errors.New("managed paths overlap")
			}
		}
	}
	return nil
}
func (r *remote) command(t Target, test bool) error {
	s, e := r.ssh.NewSession()
	if e != nil {
		return e
	}
	defer s.Close()
	cmd := "nginx -s reload -c " + quote(t.Config.ConfigurationPath)
	if test {
		cmd = "nginx -t -c " + quote(t.Config.ConfigurationPath)
	}
	if t.Config.Sudo {
		cmd = "sudo -n " + cmd
	} // Never return command output; configurations can contain secrets.
	if e = s.Run(cmd); e != nil {
		return fmt.Errorf("nginx command failed: %w", e)
	}
	return nil
}
func (r *remote) read(p string) ([]byte, error) {
	if e := r.checkParents(p); e != nil {
		return nil, e
	}
	st, e := r.files.Lstat(p)
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("managed file must be regular, not a symlink")
	}
	if st.Size() > MaxUpload {
		return nil, errors.New("remote file too large")
	}
	f, e := r.files.Open(p)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, MaxUpload+1))
}
func (r *remote) write(p string, b []byte, mode os.FileMode) error {
	if e := r.checkParents(p); e != nil {
		return e
	}
	f, e := r.files.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if e != nil {
		return e
	}
	if e = f.Chmod(mode); e != nil {
		f.Close()
		r.files.Remove(p)
		return e
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		st, se := r.files.Stat(p)
		if se != nil {
			return se
		}
		if st.Mode().Perm() != mode {
			return errors.New("remote file permissions mismatch")
		}
	}
	return e
}
func (r *remote) replace(dst, tmp string, b []byte, mode os.FileMode) error {
	if e := r.checkParents(dst); e != nil {
		return e
	}
	st, e := r.files.Lstat(dst)
	if e != nil {
		return e
	}
	if !st.Mode().IsRegular() {
		return errors.New("managed destination must be a regular file")
	}
	// Create no secret-bearing file until its containing directory is private.
	dir := tmp + ".d"
	if e := r.files.Mkdir(dir); e != nil && !os.IsExist(e) {
		return e
	}
	st, e = r.files.Lstat(dir)
	if e != nil {
		return e
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return errors.New("unsafe staging directory")
	}
	if e = r.files.Chmod(dir, 0700); e != nil {
		return e
	}
	st, e = r.files.Stat(dir)
	if e != nil || st.Mode().Perm() != 0700 {
		return errors.New("staging directory must have mode 0700")
	}
	tmp = path.Join(dir, "payload")
	r.files.Remove(tmp)
	defer func() { r.files.Remove(tmp); r.files.RemoveDirectory(dir) }()
	if e := r.write(tmp, b, mode); e != nil {
		return e
	}
	if e := r.files.PosixRename(tmp, dst); e != nil {
		r.files.Remove(tmp)
		return fmt.Errorf("atomic POSIX rename required: %w", e)
	}
	return nil
}
func jobDir(t Target, job string) string { return path.Join(t.Config.BackupPath, "certree-"+job) }
func (NginxDeployer) Deploy(ctx context.Context, t Target, b *Bundle, job string, stage func(string) error) error {
	if e := ValidateTarget(t.Config); e != nil {
		return e
	}
	r, e := connect(ctx, t)
	if e != nil {
		return e
	}
	defer r.Close()
	if e = stage("Connected"); e != nil {
		return e
	}
	dir := jobDir(t, job)
	if e = r.files.MkdirAll(t.Config.BackupPath); e != nil {
		return e
	}
	if e = r.checkParents(path.Join(t.Config.BackupPath, "probe")); e != nil {
		return e
	}
	if e = r.files.Chmod(t.Config.BackupPath, 0700); e != nil {
		return e
	}
	if e = r.files.Mkdir(dir); e != nil {
		return e
	}
	if e = r.files.Chmod(dir, 0700); e != nil {
		return e
	}
	for _, p := range []string{t.Config.CertificatePath, t.Config.KeyPath, t.Config.ConfigurationPath} {
		if _, e = r.read(p); e != nil {
			return fmt.Errorf("backup prerequisite failed: %w", e)
		}
	}
	keyInfo, e := r.files.Stat(t.Config.KeyPath)
	if e != nil {
		return e
	}
	if keyInfo.Mode().Perm()&0077 != 0 {
		return errors.New("existing private key must not be accessible to group or others")
	}
	modes := map[string]uint32{}
	for _, f := range []struct{ src, name string }{{t.Config.CertificatePath, "certificate"}, {t.Config.KeyPath, "key"}, {t.Config.ConfigurationPath, "configuration"}} {
		info, err := r.files.Stat(f.src)
		if err != nil {
			return err
		}
		modes[f.name] = uint32(info.Mode().Perm())
		data, e := r.read(f.src)
		if e != nil {
			return e
		}
		if e = r.write(path.Join(dir, f.name), data, 0600); e != nil {
			return e
		}
	}
	if e = r.write(path.Join(dir, "ready"), []byte(encode(modes)), 0600); e != nil {
		return e
	}
	if e = stage("Backed up"); e != nil {
		return e
	}
	if e = r.replace(t.Config.KeyPath, t.Config.KeyPath+".certree-"+job, b.KeyPEM, 0600); e != nil {
		return e
	}
	if e = stage("Key replaced"); e != nil {
		return e
	}
	if e = r.replace(t.Config.CertificatePath, t.Config.CertificatePath+".certree-"+job, b.PEM, 0644); e != nil {
		return e
	}
	if e = stage("Certificate replaced"); e != nil {
		return e
	}
	if e = r.command(t, true); e != nil {
		return e
	}
	if e = stage("Configuration checked"); e != nil {
		return e
	}
	if e = r.command(t, false); e != nil {
		return e
	}
	return stage("Reloaded")
}
func (NginxDeployer) Recover(ctx context.Context, t Target, job string) (string, error) {
	r, e := connect(ctx, t)
	if e != nil {
		return "", e
	}
	defer r.Close()
	defer func() {
		r.files.Remove(t.Config.CertificatePath + ".certree-" + job)
		r.files.Remove(t.Config.KeyPath + ".certree-" + job)
		for _, p := range []string{t.Config.CertificatePath, t.Config.KeyPath} {
			dir := p + ".certree-" + job + ".d"
			if r.checkParents(path.Join(dir, "payload")) == nil {
				r.files.Remove(path.Join(dir, "payload"))
				r.files.RemoveDirectory(dir)
			}
		}
	}()
	dir := jobDir(t, job)
	if _, e = r.files.Stat(path.Join(dir, "ready")); e != nil {
		if os.IsNotExist(e) {
			return "", nil
		}
		return "", e
	}
	manifest, e := r.read(path.Join(dir, "ready"))
	if e != nil {
		return "", e
	}
	var modes map[string]uint32
	if e = json.Unmarshal(manifest, &modes); e != nil {
		return "", errors.New("invalid backup manifest")
	}
	cert, e := r.read(path.Join(dir, "certificate"))
	if e != nil {
		return "", e
	}
	block, _ := pem.Decode(cert)
	if block == nil {
		return "", errors.New("backup certificate invalid")
	}
	c, e := x509.ParseCertificate(block.Bytes)
	if e != nil {
		return "", e
	}
	for _, f := range []struct {
		dst, name string
	}{{t.Config.KeyPath, "key"}, {t.Config.CertificatePath, "certificate"}, {t.Config.ConfigurationPath, "configuration"}} {
		data, e := r.read(path.Join(dir, f.name))
		if e != nil {
			return "", e
		}
		mode, ok := modes[f.name]
		if !ok || mode > 0777 {
			return "", errors.New("backup mode missing or invalid")
		}
		if f.name == "key" && mode&0077 != 0 {
			return "", errors.New("unsafe backup key mode")
		}
		// Normal deployments never edit the config. Preserve a read-only root-owned
		// configuration when it still matches the backup.
		if f.name == "configuration" {
			current, err := r.read(f.dst)
			if err == nil && bytes.Equal(current, data) {
				continue
			}
		}
		if e = r.replace(f.dst, f.dst+".certree-recover-"+job, data, os.FileMode(mode)); e != nil {
			return "", e
		}
	}
	if e = r.command(t, true); e != nil {
		return "", e
	}
	if e = r.command(t, false); e != nil {
		return "", e
	}
	return Fingerprint(c), nil
}
func (r *remote) checkParents(p string) error {
	for parent := path.Dir(p); parent != "/"; parent = path.Dir(parent) {
		st, e := r.files.Lstat(parent)
		if e != nil {
			return e
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return errors.New("remote parent directories must not be symlinks")
		}
		if st.Mode().Perm()&0022 != 0 {
			return errors.New("remote parent directories must not be writable by group or others")
		}
	}
	return nil
}
func (s *Store) Target(id string) (Target, error) {
	t := Target{ID: id}
	var sc, tc string
	var secret []byte
	e := s.DB.QueryRow(`SELECT t.certificate_id,t.endpoint_id,e.host,e.port,e.sni,s.config,s.secret,t.config FROM deployment_targets t JOIN server_endpoints e ON e.id=t.endpoint_id JOIN servers s ON s.id=e.server_id WHERE t.id=?`, id).Scan(&t.CertificateID, &t.EndpointID, &t.Host, &t.Port, &t.SNI, &sc, &secret, &tc)
	if e != nil {
		return t, e
	}
	if e = json.Unmarshal([]byte(sc), &t.Server); e != nil {
		return t, e
	}
	if e = json.Unmarshal([]byte(tc), &t.Config); e != nil {
		return t, e
	}
	t.SSHKey, e = s.Unseal(secret)
	return t, e
}
func (s *Store) Bundle(version string) (*Bundle, Validation, error) {
	var p, val string
	var enc []byte
	e := s.DB.QueryRow(`SELECT v.pem,v.validation,s.secret FROM certificate_versions v JOIN certificate_secrets s ON s.version_id=v.id WHERE v.id=?`, version).Scan(&p, &val, &enc)
	if e != nil {
		return nil, Validation{}, e
	}
	key, e := s.Unseal(enc)
	if e != nil {
		return nil, Validation{}, e
	}
	b, e := ParseBundle(append([]byte(p), key...), "")
	var v Validation
	json.Unmarshal([]byte(val), &v)
	return b, v, e
}
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum)
}
