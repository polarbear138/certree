package agent

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func guiToken(dir string) (string, error) {
	p := filepath.Join(dir, "gui-token")
	if e := safePath(p, false); e != nil {
		return "", e
	}
	b, e := os.ReadFile(p)
	if e == nil {
		return strings.TrimSpace(string(b)), nil
	}
	if !os.IsNotExist(e) {
		return "", e
	}
	token := RandomID() + RandomID()
	if e = atomicWrite(p, []byte(token+"\n"), 0600); e != nil {
		return "", e
	}
	return token, nil
}
func installService(dir string, gui bool, out io.Writer) error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return errors.New("systemd installation requires root on Linux")
	}
	if e := privateDir(dir); e != nil {
		return e
	}
	binary, e := os.Executable()
	if e != nil {
		return e
	}
	binary, e = filepath.EvalSymlinks(binary)
	if e != nil {
		return e
	}
	if strings.ContainsAny(binary+dir, " \t\r\n%\"\\") {
		return errors.New("systemd installation requires paths without whitespace, percent, quotes or backslashes")
	}
	name, command := "certree-agent", "run"
	if gui {
		name = "certree-gui"
		command = "gui serve"
		if _, e = guiToken(dir); e != nil {
			return e
		}
	}
	unit := fmt.Sprintf("[Unit]\nDescription=Certree %s\nAfter=network-online.target\nWants=network-online.target\n\n[Service]\nType=simple\nExecStart=%s --state-dir %s %s\nRestart=on-failure\nRestartSec=10\nUMask=0077\nNoNewPrivileges=true\nPrivateTmp=true\nProtectHome=true\n\n[Install]\nWantedBy=multi-user.target\n", name, binary, dir, command)
	if gui {
		unit = strings.Replace(unit, "ProtectHome=true", "ProtectHome=true\nProtectSystem=strict\nReadWritePaths="+dir, 1)
	}
	path := filepath.Join("/etc/systemd/system", name+".service")
	if e = atomicWrite(path, []byte(unit), 0644); e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if e = exec.CommandContext(ctx, "systemctl", "daemon-reload").Run(); e != nil {
		return e
	}
	fmt.Fprintf(out, "Installed %s. Start with: systemctl enable --now %s\n", path, name)
	if gui {
		fmt.Fprintf(out, "GUI: http://127.0.0.1:9473 · username: certree\nPassword is stored in %s (read locally). Use SSH port forwarding for remote access.\n", filepath.Join(dir, "gui-token"))
	}
	return nil
}

// Optional local GUI is read-only; it cannot expose credentials or initiate deployments.
func ServeGUI(ctx context.Context, dir string, out io.Writer) error {
	if e := privateDir(dir); e != nil {
		return e
	}
	token, e := guiToken(dir)
	if e != nil {
		return e
	}
	listener, e := net.Listen("tcp", "127.0.0.1:9473")
	if e != nil {
		return e
	}
	server := &http.Server{Handler: guiHandler(dir, token), ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	fmt.Fprintf(out, "GUI listening at http://127.0.0.1:9473 · username: certree · password file: %s\n", filepath.Join(dir, "gui-token"))
	e = server.Serve(listener)
	if errors.Is(e, http.ErrServerClosed) {
		return nil
	}
	return e
}

func guiHandler(dir, token string) http.Handler {
	tpl := template.Must(template.New("gui").Parse(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Certree Agent</title><style>body{font:16px system-ui;background:#f2f6f3;color:#163c2a;margin:40px auto;padding:0 24px;max-width:1000px}h1{font-size:32px}section{background:white;border:1px solid #d5e2da;border-radius:12px;padding:24px;margin:20px 0}pre{white-space:pre-wrap;overflow-wrap:anywhere;font-size:13px}a{color:#176e4b}</style><h1>Certree Agent</h1><p>Local status and execution history. Final TLS verification is available in the central console.</p><a href="/">Refresh</a><section><h2>Status</h2><pre>{{.Status}}</pre></section><section><h2>Local history</h2><pre>{{.History}}</pre></section></html>`))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'")
		if r.Host != "127.0.0.1:9473" && r.Host != "localhost:9473" {
			http.Error(w, "invalid host", 403)
			return
		}
		user, password, ok := r.BasicAuth()
		if !ok || user != "certree" || subtle.ConstantTimeCompare([]byte(password), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Certree local agent"`)
			http.Error(w, "authentication required", 401)
			return
		}
		if r.Method != "GET" || r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		status, e := Status(dir)
		if e != nil {
			http.Error(w, "status unavailable", 500)
			return
		}
		history, e := LocalHistory(dir)
		if e != nil {
			http.Error(w, "history unavailable", 500)
			return
		}
		var s, h strings.Builder
		printJSON(&s, status)
		printJSON(&h, history)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tpl.Execute(w, map[string]string{"Status": s.String(), "History": h.String()})
	})
}
