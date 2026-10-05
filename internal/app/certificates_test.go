package app

import (
	"archive/zip"
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"github.com/certree/certree/internal/testkit"
	"os"
	"path/filepath"
	pkcs12 "software.sslmate.com/src/go-pkcs12"
	"testing"
)

func fixture(t *testing.T) testkit.Fixture {
	t.Helper()
	f, e := testkit.Generate()
	if e != nil {
		t.Fatal(e)
	}
	return f
}
func store(t *testing.T) *Store {
	t.Helper()
	s, e := Open(filepath.Join(t.TempDir(), "test.db"), base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func roots(f testkit.Fixture) *x509.CertPool { p := x509.NewCertPool(); p.AddCert(f.CA); return p }
func TestCertificateParsingAndValidation(t *testing.T) {
	f := fixture(t)
	b, e := ParseBundle(f.First, "")
	if e != nil {
		t.Fatal(e)
	}
	if !b.Validate(roots(f), "lab.test").Valid {
		t.Fatal("trusted leaf rejected")
	}
	if !b.Validate(roots(f), "api.example.test").Valid {
		t.Fatal("wildcard matching failed")
	}
	for _, domain := range []string{"deep.api.example.test", "example.test", "unrelated.test"} {
		if b.Validate(roots(f), domain).Valid {
			t.Fatalf("accepted invalid hostname %s", domain)
		}
	}
	if b.Validate(x509.NewCertPool(), "lab.test").Valid {
		t.Fatal("untrusted chain accepted")
	}
	for _, bundle := range [][]byte{f.Expired, f.Future} {
		b, e := ParseBundle(bundle, "")
		if e != nil {
			t.Fatal(e)
		}
		if b.Validate(roots(f), "lab.test").Valid {
			t.Fatal("invalid lifetime accepted")
		}
	}
	other, _ := ParseBundle(f.Second, "")
	if _, e := ParseBundle(append(b.PEM, other.KeyPEM...), ""); e == nil {
		t.Fatal("mismatched key accepted")
	}
	if _, e := ParseBundle([]byte("not a certificate"), ""); e == nil {
		t.Fatal("garbage accepted")
	}
}
func TestFormatsAndArchiveDefenses(t *testing.T) {
	f := fixture(t)
	b, e := ParseBundle(f.First, "")
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"../escape.key", "/absolute.pem", "C:/drive.pem", "folder\\..\\escape.key"} {
		var buf bytes.Buffer
		z := zip.NewWriter(&buf)
		w, _ := z.Create(name)
		w.Write(f.First)
		z.Close()
		if _, e := ParseBundle(buf.Bytes(), ""); e == nil {
			t.Fatalf("unsafe archive accepted %s", name)
		}
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: "link.pem"}
	h.SetMode(os.ModeSymlink | 0777)
	w, _ := z.CreateHeader(h)
	w.Write(f.First)
	z.Close()
	if _, e := ParseBundle(buf.Bytes(), ""); e == nil {
		t.Fatal("symlink accepted")
	}
	buf.Reset()
	z = zip.NewWriter(&buf)
	w, _ = z.Create("bundle.pem")
	w.Write(f.First)
	z.Close()
	valid := append([]byte{}, buf.Bytes()...)
	if _, e := ParseBundle(valid, ""); e != nil {
		t.Fatal(e)
	}
	buf.Reset()
	z = zip.NewWriter(&buf)
	w, _ = z.Create("nested.zip")
	w.Write(valid)
	z.Close()
	if _, e := ParseBundle(buf.Bytes(), ""); e == nil {
		t.Fatal("nested ZIP accepted")
	}
	if _, e := ParseBundle(make([]byte, MaxUpload+1), ""); e == nil {
		t.Fatal("oversized upload accepted")
	}
	buf.Reset()
	z = zip.NewWriter(&buf)
	w, _ = z.Create("bomb")
	w.Write(make([]byte, MaxUpload+1))
	z.Close()
	if _, e := ParseBundle(buf.Bytes(), ""); e == nil {
		t.Fatal("expansion bomb accepted")
	}
	pfx, e := pkcs12.Modern.Encode(b.Key, b.Leaf, b.Chain, "secret")
	if e != nil {
		t.Fatal(e)
	}
	if _, e := ParseBundle(pfx, "secret"); e != nil {
		t.Fatal(e)
	}
	if _, e := ParseBundle(pfx, "wrong"); e == nil {
		t.Fatal("bad PFX password accepted")
	}
	der := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: mustPKCS8(t, b.Key)})
	var derbuf bytes.Buffer
	z = zip.NewWriter(&derbuf)
	w, _ = z.Create("certificate.cer")
	w.Write(b.Leaf.Raw)
	w, _ = z.Create("private.key")
	w.Write(der)
	w, _ = z.Create("ca.pem")
	w.Write(f.CAPEM)
	z.Close()
	if _, e := ParseBundle(derbuf.Bytes(), ""); e != nil {
		t.Fatal(e)
	}
}
func mustPKCS8(t *testing.T, k any) []byte {
	b, e := x509.MarshalPKCS8PrivateKey(k)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestEncryptedPersistenceAndVersions(t *testing.T) {
	s := store(t)
	f := fixture(t)
	s.DB.Exec("INSERT INTO organizations VALUES('o','Org')")
	s.DB.Exec("INSERT INTO environments VALUES('e','o','Production')")
	first, e := s.Import("e", f.First, "", roots(f))
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Import("e", f.First, "", roots(f))
	if e != nil || first != again {
		t.Fatal("duplicate import not idempotent")
	}
	second, e := s.Import("e", f.Second, "", roots(f))
	if e != nil || first == second {
		t.Fatal("renewal did not create version")
	}
	var count int
	s.DB.QueryRow("SELECT count(*) FROM certificates").Scan(&count)
	if count != 1 {
		t.Fatal("renewal duplicated entity")
	}
	s.DB.QueryRow("SELECT count(*) FROM certificate_versions").Scan(&count)
	if count != 2 {
		t.Fatal("version history lost")
	}
	var secret []byte
	s.DB.QueryRow("SELECT secret FROM certificate_secrets WHERE version_id=?", first).Scan(&secret)
	if bytes.Contains(secret, []byte("PRIVATE KEY")) {
		t.Fatal("key stored unencrypted")
	}
	plain, e := s.Unseal(secret)
	if e != nil || !bytes.Contains(plain, []byte("PRIVATE KEY")) {
		t.Fatal("secret roundtrip failed")
	}
	secret[len(secret)-1] ^= 1
	if _, e = s.Unseal(secret); e == nil {
		t.Fatal("tampering not detected")
	}
	rows, e := s.Rows("SELECT metadata,validation FROM certificate_versions")
	if e != nil {
		t.Fatal(e)
	}
	for _, r := range rows {
		if bytes.Contains([]byte(encode(r)), []byte("PRIVATE KEY")) {
			t.Fatal("private key in public metadata")
		}
	}
}
