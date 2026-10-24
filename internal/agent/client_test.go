package agent

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestClientTLSAuthenticationAndRedirectRejection(t *testing.T) {
	token := RandomID() + RandomID()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("missing credential")
		}
		if r.URL.Path == "/api/agent/v1/redirect" {
			http.Redirect(w, r, "https://other.invalid", 302)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	c, e := NewClient(Config{Server: server.URL, CAFile: ca, Token: token})
	if e != nil {
		t.Fatal(e)
	}
	var reply map[string]bool
	if e = c.Request(context.Background(), "GET", "test", nil, &reply); e != nil || !reply["ok"] {
		t.Fatal(e)
	}
	if e = c.Request(context.Background(), "GET", "redirect", nil, nil); e == nil {
		t.Fatal("redirect followed")
	}
	untrusted, e := NewClient(Config{Server: server.URL, Token: token})
	if e != nil {
		t.Fatal(e)
	}
	if e = untrusted.Request(context.Background(), "GET", "test", nil, nil); e == nil {
		t.Fatal("untrusted server accepted")
	}
}
