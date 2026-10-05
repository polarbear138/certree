// Package testkit generates isolated trust material for tests and the local lab.
package testkit

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"golang.org/x/crypto/ssh"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

type Fixture struct {
	CA                                                                                                                *x509.Certificate
	CAPEM, CAKeyPEM, First, Second, Expired, Future, SSHKey, SSHPublic, HostKey, HostPublic, MailCertificate, MailKey []byte
}

func cert(t *x509.Certificate, parent *x509.Certificate, pub any, key any) ([]byte, error) {
	return x509.CreateCertificate(rand.Reader, t, parent, pub, key)
}
func private(k any) []byte {
	b, _ := x509.MarshalPKCS8PrivateKey(k)
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: b})
}
func certificate(b []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: b})
}
func Generate() (Fixture, error) {
	f := Fixture{}
	k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return f, e
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Certree isolated test CA"}, NotBefore: now.Add(-365 * 24 * time.Hour), NotAfter: now.Add(365 * 24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	raw, e := cert(ca, ca, k.Public(), k)
	if e != nil {
		return f, e
	}
	f.CA, _ = x509.ParseCertificate(raw)
	f.CAPEM = certificate(raw)
	f.CAKeyPEM = private(k)
	makeLeaf := func(serial int64, before, after time.Time, names []string) ([]byte, []byte, error) {
		key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			return nil, nil, e
		}
		tpl := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: names[0]}, DNSNames: names, NotBefore: before, NotAfter: after, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature}
		b, e := cert(tpl, f.CA, key.Public(), k)
		return certificate(b), private(key), e
	}
	first, key, e := makeLeaf(2, now.Add(-time.Hour), now.Add(60*24*time.Hour), []string{"lab.test", "*.example.test"})
	if e != nil {
		return f, e
	}
	f.First = append(append(first, key...), f.CAPEM...)
	second, key, e := makeLeaf(3, now.Add(-time.Hour), now.Add(90*24*time.Hour), []string{"lab.test", "*.example.test"})
	if e != nil {
		return f, e
	}
	f.Second = append(append(second, key...), f.CAPEM...)
	expired, key, e := makeLeaf(4, now.Add(-10*24*time.Hour), now.Add(-time.Hour), []string{"lab.test", "*.example.test"})
	if e != nil {
		return f, e
	}
	f.Expired = append(append(expired, key...), f.CAPEM...)
	future, key, e := makeLeaf(5, now.Add(24*time.Hour), now.Add(90*24*time.Hour), []string{"lab.test", "*.example.test"})
	if e != nil {
		return f, e
	}
	f.Future = append(append(future, key...), f.CAPEM...)
	f.MailCertificate, f.MailKey, e = makeLeaf(6, now.Add(-time.Hour), now.Add(60*24*time.Hour), []string{"localhost", "mail"})
	if e != nil {
		return f, e
	}
	f.MailCertificate = append(f.MailCertificate, f.CAPEM...)
	pub, sk, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return f, e
	}
	b, _ := ssh.MarshalPrivateKey(sk, "certree-test-client")
	f.SSHKey = pem.EncodeToMemory(b)
	sp, _ := ssh.NewPublicKey(pub)
	f.SSHPublic = ssh.MarshalAuthorizedKey(sp)
	pub, hk, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return f, e
	}
	b, _ = ssh.MarshalPrivateKey(hk, "certree-test-host")
	f.HostKey = pem.EncodeToMemory(b)
	hp, _ := ssh.NewPublicKey(pub)
	f.HostPublic = ssh.MarshalAuthorizedKey(hp)
	return f, nil
}
func (f Fixture) Write(dir string) error {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	for name, b := range map[string][]byte{"ca.pem": f.CAPEM, "ca.key": f.CAKeyPEM, "v1.pem": f.First, "v2.pem": f.Second, "expired.pem": f.Expired, "future.pem": f.Future, "ssh.key": f.SSHKey, "authorized_keys": f.SSHPublic, "ssh_host_ed25519_key": f.HostKey, "host.pub": f.HostPublic, "mail.pem": f.MailCertificate, "mail.key": f.MailKey} {
		if e := os.WriteFile(filepath.Join(dir, name), b, 0600); e != nil {
			return e
		}
	}
	return nil
}
