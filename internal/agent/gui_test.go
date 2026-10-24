package agent

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestGUIAuthenticationHostAndReadOnly(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	if e := Save(dir, Config{Server: "https://example.test", ID: "test", Token: "agent-secret"}); e != nil {
		t.Fatal(e)
	}
	h := guiHandler(dir, "gui-secret")
	for _, tc := range []struct {
		method, host, password string
		status                 int
	}{
		{"GET", "127.0.0.1:9473", "", 401},
		{"GET", "attacker.invalid:9473", "gui-secret", 403},
		{"POST", "127.0.0.1:9473", "gui-secret", 404},
		{"GET", "127.0.0.1:9473", "gui-secret", 200},
	} {
		r := httptest.NewRequest(tc.method, "http://"+tc.host+"/", nil)
		if tc.password != "" {
			r.SetBasicAuth("certree", tc.password)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%+v: got %d", tc, w.Code)
		}
		if strings.Contains(w.Body.String(), "agent-secret") || strings.Contains(w.Body.String(), "gui-secret") {
			t.Fatal("GUI leaked a credential")
		}
	}
}
