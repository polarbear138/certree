// Package agent implements the outbound-only Certree client.
package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/certree/certree/internal/agentwire"
)

const Version = "0.2.0-dev"

type Config struct {
	Server  string             `json:"server"`
	CAFile  string             `json:"ca_file,omitempty"`
	ID      string             `json:"id"`
	Token   string             `json:"token"`
	Targets []agentwire.Target `json:"targets"`
}
type Client struct {
	Config Config
	HTTP   *http.Client
}

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("server HTTP %d: %s", e.Status, e.Message) }

func RandomID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func NewClient(c Config) (*Client, error) {
	u, e := url.Parse(c.Server)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("server must be an HTTPS origin, for example https://certree.example.com")
	}
	roots, e := x509.SystemCertPool()
	if e != nil {
		roots = x509.NewCertPool()
	}
	if c.CAFile != "" {
		b, e := os.ReadFile(c.CAFile)
		if e != nil {
			return nil, e
		}
		if !roots.AppendCertsFromPEM(b) {
			return nil, errors.New("CA file contains no certificates")
		}
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
	return &Client{c, &http.Client{Transport: tr, Timeout: 50 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirects are not allowed") }}}, nil
}
func (c *Client) Request(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return e
		}
		reader = bytes.NewReader(b)
	}
	r, e := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.Config.Server, "/")+"/api/agent/v1/"+path, reader)
	if e != nil {
		return e
	}
	r.Header.Set("Content-Type", "application/json")
	if c.Config.Token != "" {
		r.Header.Set("Authorization", "Bearer "+c.Config.Token)
	}
	resp, e := c.HTTP.Do(r)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if e != nil {
		return e
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var v struct {
			Error string `json:"error"`
		}
		json.Unmarshal(b, &v)
		return &APIError{Status: resp.StatusCode, Message: v.Error}
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}
func Load(dir string) (Config, error) {
	var c Config
	if e := safePath(filepath.Join(dir, "config.json"), true); e != nil {
		return c, e
	}
	st, e := os.Stat(filepath.Join(dir, "config.json"))
	if e != nil {
		return c, e
	}
	if !st.Mode().IsRegular() || st.Size() > 1<<20 || (runtime.GOOS != "windows" && st.Mode().Perm()&0077 != 0) {
		return c, errors.New("config must be a regular file of at most 1 MiB, with mode 0600 or stricter")
	}
	b, e := os.ReadFile(filepath.Join(dir, "config.json"))
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(b, &c)
	return c, e
}
func Save(dir string, c Config) error {
	if e := privateDir(dir); e != nil {
		return e
	}
	return writeJSON(filepath.Join(dir, "config.json"), c)
}
func writeJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return atomicWrite(path, append(b, '\n'), 0600)
}

// Run serializes jobs and retries durable results until the server acknowledges them.
func Run(ctx context.Context, dir string) error {
	if e := privateDir(dir); e != nil {
		return e
	}
	unlock, e := acquire(dir)
	if e != nil {
		return e
	}
	defer unlock()
	cfg, e := Load(dir)
	if e != nil {
		return e
	}
	client, e := NewClient(cfg)
	if e != nil {
		return e
	}
	executor := Executor{Dir: dir, Targets: cfg.Targets, Command: runCommand}
	logEvent(dir, "agent started")
	delay := time.Second
	for {
		e = func() error {
			for _, t := range cfg.Targets {
				if err := client.Request(ctx, "POST", "targets", t, nil); err != nil {
					return err
				}
			}
			var reply struct {
				Task *agentwire.Task `json:"task"`
			}
			if err := client.Request(ctx, "POST", "poll", struct{}{}, &reply); err != nil {
				return err
			}
			if reply.Task == nil {
				return nil
			}
			result, err := executor.Execute(ctx, *reply.Task)
			if err != nil {
				return err
			}
			logEvent(dir, "job "+reply.Task.ID+" "+result.Phase+": "+result.Outcome)
			return client.Request(ctx, "POST", "jobs/"+reply.Task.ID+"/result", result, nil)
		}()
		state := map[string]any{"checked_at": time.Now().UTC(), "connected": e == nil, "agent_id": cfg.ID, "targets": len(cfg.Targets)}
		if e != nil {
			state["error"] = e.Error()
			logEvent(dir, e.Error())
			delay *= 2
			if delay > time.Minute {
				delay = time.Minute
			}
		} else {
			delay = 10 * time.Second
		}
		if err := writeJSON(filepath.Join(dir, "status.json"), state); err != nil {
			return err
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
func logEvent(dir, message string) {
	p := filepath.Join(dir, "agent.log")
	if safePath(p, false) != nil {
		return
	}
	if s, e := os.Stat(p); e == nil && s.Size() > 4<<20 {
		os.Rename(p, p+".1")
	}
	f, e := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, time.Now().UTC().Format(time.RFC3339), strings.ReplaceAll(strings.ReplaceAll(message, "\n", " "), "\r", " "))
}
