package app

import (
	"archive/zip"
	"bytes"
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"path"
	pkcs12 "software.sslmate.com/src/go-pkcs12"
	"sort"
	"strings"
	"time"
)

const MaxUpload = 16 << 20

type Bundle struct {
	Leaf        *x509.Certificate
	Chain       []*x509.Certificate
	Key         crypto.Signer
	PEM, KeyPEM []byte
}
type Validation struct {
	Valid    bool     `json:"valid"`
	KeyMatch bool     `json:"key_match"`
	Trusted  bool     `json:"trusted"`
	Errors   []string `json:"errors"`
}

func Fingerprint(c *x509.Certificate) string {
	h := sha256.Sum256(c.Raw)
	return hex.EncodeToString(h[:])
}
func ParseBundle(data []byte, password string) (*Bundle, error) {
	if len(data) > MaxUpload {
		return nil, errors.New("upload exceeds 16 MiB")
	}
	var certs []*x509.Certificate
	var keys []crypto.Signer
	count, total := 0, 0
	var parse func([]byte, int) error
	parse = func(b []byte, depth int) error {
		total += len(b)
		if total > MaxUpload*2 {
			return errors.New("expanded bundle too large")
		}
		if depth > 1 {
			return errors.New("nested archives are not supported")
		}
		if bytes.HasPrefix(b, []byte("PK\x03\x04")) {
			z, e := zip.NewReader(bytes.NewReader(b), int64(len(b)))
			if e != nil {
				return e
			}
			if len(z.File) > 64 {
				return errors.New("too many archive entries")
			}
			for _, f := range z.File {
				if f.FileInfo().IsDir() {
					continue
				}
				name := strings.ReplaceAll(f.Name, "\\", "/")
				if path.IsAbs(name) || strings.Contains(name, ":") || path.Clean(name) != name || strings.HasPrefix(name, "../") {
					return errors.New("unsafe ZIP path")
				}
				if !f.Mode().IsRegular() {
					return errors.New("non-regular ZIP entry")
				}
				if f.UncompressedSize64 > MaxUpload {
					return errors.New("ZIP entry too large")
				}
				r, e := f.Open()
				if e != nil {
					return e
				}
				v, e := io.ReadAll(io.LimitReader(r, MaxUpload+1))
				r.Close()
				if e != nil {
					return e
				}
				if e = parse(v, depth+1); e != nil {
					return fmt.Errorf("archive entry rejected: %w", e)
				}
			}
			return nil
		}
		count++
		if count > 64 {
			return errors.New("too many files")
		}
		remaining := b
		found := false
		for {
			block, rest := pem.Decode(remaining)
			if block == nil {
				break
			}
			found = true
			remaining = rest
			switch block.Type {
			case "CERTIFICATE":
				cs, e := x509.ParseCertificates(block.Bytes)
				if e != nil {
					return e
				}
				certs = append(certs, cs...)
			case "PRIVATE KEY", "RSA PRIVATE KEY", "EC PRIVATE KEY":
				var k any
				var e error
				switch block.Type {
				case "PRIVATE KEY":
					k, e = x509.ParsePKCS8PrivateKey(block.Bytes)
				case "RSA PRIVATE KEY":
					k, e = x509.ParsePKCS1PrivateKey(block.Bytes)
				case "EC PRIVATE KEY":
					k, e = x509.ParseECPrivateKey(block.Bytes)
				}
				if e != nil {
					return errors.New("invalid private key")
				}
				s, ok := k.(crypto.Signer)
				if !ok {
					return errors.New("unsupported private key")
				}
				keys = append(keys, s)
			default:
				return fmt.Errorf("unsupported PEM block: %s", block.Type)
			}
		}
		if found {
			if len(bytes.TrimSpace(remaining)) != 0 {
				return errors.New("trailing data in PEM")
			}
			return nil
		}
		if c, e := x509.ParseCertificates(b); e == nil {
			certs = append(certs, c...)
			return nil
		}
		for _, parser := range []func([]byte) (any, error){x509.ParsePKCS8PrivateKey, func(b []byte) (any, error) { return x509.ParsePKCS1PrivateKey(b) }, func(b []byte) (any, error) { return x509.ParseECPrivateKey(b) }} {
			if k, err := parser(b); err == nil {
				if signer, ok := k.(crypto.Signer); ok {
					keys = append(keys, signer)
					return nil
				}
			}
		}
		k, c, chain, e := pkcs12.DecodeChain(b, password)
		if e != nil {
			return errors.New("file is not PEM, DER, PKCS#12 or ZIP")
		}
		s, ok := k.(crypto.Signer)
		if !ok {
			return errors.New("PKCS#12 lacks supported private key")
		}
		keys = append(keys, s)
		certs = append(certs, c)
		certs = append(certs, chain...)
		return nil
	}
	if e := parse(data, 0); e != nil {
		return nil, e
	}
	if len(keys) != 1 {
		return nil, errors.New("exactly one private key required")
	}
	var leaf *x509.Certificate
	chain := []*x509.Certificate{}
	pub, _ := x509.MarshalPKIXPublicKey(keys[0].Public())
	for _, c := range certs {
		cp, _ := x509.MarshalPKIXPublicKey(c.PublicKey)
		if bytes.Equal(pub, cp) && !c.IsCA {
			if leaf != nil && Fingerprint(leaf) != Fingerprint(c) {
				return nil, errors.New("multiple matching leaf certificates")
			}
			leaf = c
		} else {
			chain = append(chain, c)
		}
	}
	if leaf == nil {
		return nil, errors.New("certificate and private key do not match")
	}
	// Order intermediates by signature, independent of upload order.
	ordered := []*x509.Certificate{}
	current := leaf
	for len(chain) > 0 {
		idx := -1
		for i, c := range chain {
			if current.CheckSignatureFrom(c) == nil {
				idx = i
				break
			}
		}
		if idx < 0 {
			return nil, errors.New("unrelated certificate in bundle")
		}
		next := chain[idx]
		ordered = append(ordered, next)
		chain = append(chain[:idx], chain[idx+1:]...)
		current = next
	}
	out := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw})
	for _, c := range ordered {
		out = append(out, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})...)
	}
	kp, e := x509.MarshalPKCS8PrivateKey(keys[0])
	if e != nil {
		return nil, e
	}
	return &Bundle{leaf, ordered, keys[0], out, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kp})}, nil
}
func (b *Bundle) Validate(roots *x509.CertPool, domain string) Validation {
	v := Validation{KeyMatch: true, Errors: []string{}}
	pool := x509.NewCertPool()
	for _, c := range b.Chain {
		pool.AddCert(c)
	}
	_, e := b.Leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: pool, DNSName: domain, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	v.Trusted = e == nil
	if e != nil {
		v.Errors = append(v.Errors, e.Error())
	}
	if len(b.Leaf.DNSNames) == 0 {
		v.Errors = append(v.Errors, "SAN DNS names required")
	}
	v.Valid = len(v.Errors) == 0
	return v
}
func (b *Bundle) Metadata() map[string]any {
	return map[string]any{"subject": b.Leaf.Subject.String(), "issuer": b.Leaf.Issuer.String(), "serial": b.Leaf.SerialNumber.String(), "sans": b.Leaf.DNSNames, "not_before": b.Leaf.NotBefore, "not_after": b.Leaf.NotAfter, "fingerprint": Fingerprint(b.Leaf), "public_key_algorithm": b.Leaf.PublicKeyAlgorithm.String(), "signature_algorithm": b.Leaf.SignatureAlgorithm.String(), "chain_length": len(b.Chain)}
}
func (s *Store) Import(environment string, data []byte, password string, roots *x509.CertPool) (string, error) {
	b, e := ParseBundle(data, password)
	if e != nil {
		return "", e
	}
	v := b.Validate(roots, "")
	names := append([]string{}, b.Leaf.DNSNames...)
	sort.Strings(names)
	name := strings.Join(names, ", ")
	if name == "" {
		name = b.Leaf.Subject.CommonName
	}
	tx, e := s.DB.Begin()
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	cid := ID()
	if _, e = tx.Exec("INSERT OR IGNORE INTO certificates(id,name,environment_id) VALUES(?,?,?)", cid, name, environment); e != nil {
		return "", e
	}
	if e = tx.QueryRow("SELECT id FROM certificates WHERE environment_id=? AND name=?", environment, name).Scan(&cid); e != nil {
		return "", e
	}
	var existing string
	if tx.QueryRow("SELECT id FROM certificate_versions WHERE certificate_id=? AND fingerprint=?", cid, Fingerprint(b.Leaf)).Scan(&existing) == nil {
		return existing, nil
	}
	vid := ID()
	_, e = tx.Exec("INSERT INTO certificate_versions(id,certificate_id,fingerprint,metadata,pem,validation,created_at) VALUES(?,?,?,?,?,?,?)", vid, cid, Fingerprint(b.Leaf), encode(b.Metadata()), string(b.PEM), encode(v), time.Now().UTC().Format(timestampFormat))
	if e != nil {
		return "", e
	}
	if _, e = tx.Exec("INSERT INTO certificate_secrets VALUES(?,?)", vid, s.Seal(b.KeyPEM)); e != nil {
		return "", e
	}
	chain := []byte{}
	for _, c := range b.Chain {
		chain = append(chain, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})...)
	}
	if _, e = tx.Exec("INSERT INTO certificate_chains VALUES(?,?)", vid, string(chain)); e != nil {
		return "", e
	}
	return vid, tx.Commit()
}
