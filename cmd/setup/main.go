// setup creates local installation secrets without overwriting an existing file.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
)

func token(n int) string {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return base64.StdEncoding.EncodeToString(b)
}
func main() {
	dev := flag.Bool("dev", false, "loopback-only HTTP development configuration")
	out := flag.String("out", ".env", "output environment file")
	flag.Parse()
	f, e := os.OpenFile(*out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	origin, address, mode := "https://localhost:8443", "0.0.0.0:8080", "false"
	if *dev {
		origin, address, mode = "http://127.0.0.1:8080", "127.0.0.1:8080", "true"
	}
	_, e = fmt.Fprintf(f, "CERTREE_MASTER_KEY=%s\nCERTREE_ADMIN_USER=admin\nCERTREE_ADMIN_PASSWORD=%s\nCERTREE_ORIGIN=%s\nCERTREE_ADDRESS=%s\nCERTREE_DEV_HTTP=%s\n", token(32), token(24), origin, address, mode)
	if closeErr := f.Close(); e == nil {
		e = closeErr
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	fmt.Println("Created installation configuration. Read your initial administrator credentials from", *out, "and restrict access to this file.")
}
