// lab is an isolated SMTP/IMAP fixture service. Never deploy it in production.
package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"github.com/certree/certree/internal/testkit"
	"github.com/emersion/go-imap/backend/memory"
	"github.com/emersion/go-imap/client"
	imapserver "github.com/emersion/go-imap/server"
	"io"
	"log"
	"net"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	generate := flag.String("generate", "", "generate isolated fixtures and exit")
	dir := flag.String("fixtures", "/fixtures", "fixture directory")
	imapAddress := flag.String("imap", "0.0.0.0:993", "IMAP TLS listener")
	smtpAddress := flag.String("smtp", "0.0.0.0:1025", "local test SMTP listener")
	flag.Parse()
	if *generate != "" {
		f, e := testkit.Generate()
		if e == nil {
			e = f.Write(*generate)
		}
		if e != nil {
			log.Fatal(e)
		}
		return
	}
	cert, e := tls.LoadX509KeyPair(filepath.Join(*dir, "mail.pem"), filepath.Join(*dir, "mail.key"))
	if e != nil {
		log.Fatal(e)
	}
	ln, e := tls.Listen("tcp", *imapAddress, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	if e != nil {
		log.Fatal(e)
	}
	server := imapserver.New(memory.New())
	go func() {
		if e := server.Serve(ln); e != nil {
			log.Fatal(e)
		}
	}()
	ca, e := os.ReadFile(filepath.Join(*dir, "ca.pem"))
	if e != nil {
		log.Fatal(e)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(ca)
	smtp, e := net.Listen("tcp", *smtpAddress)
	if e != nil {
		log.Fatal(e)
	}
	fmt.Println("Isolated lab mail service ready")
	for {
		conn, e := smtp.Accept()
		if e != nil {
			log.Fatal(e)
		} // One SMTP transaction at a time; send before polling IMAP.
		handle(conn, roots, strings.Replace(*imapAddress, "0.0.0.0", "localhost", 1))
	}
}
func handle(conn net.Conn, roots *x509.CertPool, address string) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(20 * time.Second))
	r := textproto.NewReader(bufio.NewReader(conn))
	write := func(s string) { io.WriteString(conn, s+"\r\n") }
	write("220 certree-lab ESMTP")
	for {
		line, e := r.ReadLine()
		if e != nil {
			return
		}
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			write("250 certree-lab")
		case strings.HasPrefix(upper, "MAIL FROM:"), strings.HasPrefix(upper, "RCPT TO:"), upper == "RSET":
			write("250 OK")
		case upper == "DATA":
			write("354 Send message")
			body, e := io.ReadAll(io.LimitReader(r.DotReader(), (16<<20)+1))
			if e != nil || len(body) > 16<<20 {
				write("552 Too large")
				return
			}
			cl, e := client.DialTLS(address, &tls.Config{ServerName: "localhost", RootCAs: roots, MinVersion: tls.VersionTLS12})
			if e == nil {
				e = cl.Login("username", "password")
			}
			if e == nil {
				e = cl.Append("INBOX", nil, time.Now(), bytes.NewReader(body))
			}
			if cl != nil {
				cl.Logout()
			}
			if e != nil {
				write("451 Mailbox unavailable")
			} else {
				write("250 Queued")
			}
		case upper == "QUIT":
			write("221 Bye")
			return
		default:
			write("502 Unsupported")
		}
	}
}
