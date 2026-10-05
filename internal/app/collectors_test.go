package app

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"github.com/emersion/go-imap/backend/memory"
	imapserver "github.com/emersion/go-imap/server"
	"net"
	"strings"
	"testing"
	"time"
)

func emailAttachment(bundle []byte) []byte {
	return []byte("From: supplier@example.test\r\nTo: certree@example.test\r\nSubject: TLS renewal\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=certree-test\r\n\r\n--certree-test\r\nContent-Type: application/octet-stream\r\nContent-Disposition: attachment; filename=certificate.pem\r\nContent-Transfer-Encoding: base64\r\n\r\n" + base64.StdEncoding.EncodeToString(bundle) + "\r\n--certree-test--\r\n")
}
func TestRealIMAPTLSCollectionAndDeduplication(t *testing.T) {
	f := fixture(t)
	be := memory.New()
	u, e := be.Login(nil, "username", "password")
	if e != nil {
		t.Fatal(e)
	}
	mb, e := u.GetMailbox("INBOX")
	if e != nil {
		t.Fatal(e)
	}
	body := emailAttachment(f.First)
	if e = mb.CreateMessage(nil, time.Now(), bytes.NewReader(body)); e != nil {
		t.Fatal(e)
	}
	pair, e := tls.X509KeyPair(f.MailCertificate, f.MailKey)
	if e != nil {
		t.Fatal(e)
	}
	ln, e := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12})
	if e != nil {
		t.Fatal(e)
	}
	server := imapserver.New(be)
	done := make(chan struct{})
	go func() { server.Serve(ln); close(done) }()
	t.Cleanup(func() { server.Close(); ln.Close(); <-done })
	a := testApp(t, f)
	a.Store.DB.Exec("INSERT INTO organizations VALUES('o','Org')")
	a.Store.DB.Exec("INSERT INTO environments VALUES('e','o','Production')")
	cfg := CollectorConfig{Host: "localhost", Port: ln.Addr().(*net.TCPAddr).Port, Username: "username", Mailbox: "INBOX", From: "supplier@example.test", Subject: "TLS renewal", Interval: 60}
	_, e = a.Store.DB.Exec("INSERT INTO collectors(id,name,environment_id,config,secret) VALUES('collector','Mail','e',?,?)", encode(cfg), a.Store.Seal([]byte("password")))
	if e != nil {
		t.Fatal(e)
	}
	a.PollDue(context.Background())
	var count, approved int
	a.Store.DB.QueryRow("SELECT count(*) FROM certificate_versions").Scan(&count)
	if count != 1 {
		var message string
		a.Store.DB.QueryRow("SELECT last_error FROM collectors").Scan(&message)
		t.Fatal("mail import failed", message)
	}
	a.Store.DB.QueryRow("SELECT approved FROM certificate_versions").Scan(&approved)
	if approved != 0 {
		t.Fatal("email auto-approved deployment")
	}
	a.Store.DB.Exec("UPDATE collectors SET next_poll=0")
	a.PollDue(context.Background())
	a.Store.DB.QueryRow("SELECT count(*) FROM collector_events").Scan(&count)
	if count != 1 {
		t.Fatal("email deduplication failed")
	}
	cfg.Username = "invalid"
	a.Store.DB.Exec("UPDATE collectors SET config=?,next_poll=0", encode(cfg))
	a.PollDue(context.Background())
	var delay int64
	var err string
	a.Store.DB.QueryRow("SELECT next_poll,last_error FROM collectors").Scan(&delay, &err)
	if delay <= time.Now().Unix() || err == "" {
		t.Fatal("retry backoff missing")
	}
}
func TestMIMEAttachmentValidation(t *testing.T) {
	f := fixture(t)
	out, e := attachments(emailAttachment(f.First))
	if e != nil || len(out) != 1 {
		t.Fatal(e)
	}
	if _, e := ParseBundle(out[0], ""); e != nil {
		t.Fatal(e)
	}
	if _, e := attachments([]byte("malformed")); e == nil {
		t.Fatal("malformed MIME accepted")
	}
	parts := []string{"From: test@example.test\r\nContent-Type: multipart/mixed; boundary=x\r\n\r\n"}
	for i := 0; i < 65; i++ {
		parts = append(parts, fmt.Sprintf("--x\r\nContent-Disposition: attachment; filename=a%d.key\r\n\r\nx\r\n", i))
	}
	parts = append(parts, "--x--\r\n")
	if _, e := attachments([]byte(strings.Join(parts, ""))); e == nil {
		t.Fatal("excess MIME parts accepted")
	}
}
