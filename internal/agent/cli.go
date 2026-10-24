package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/certree/certree/internal/agentwire"
)

const help = `Certree Agent
  certree                         Interactive menu
  certree settings                Enroll or add a local Nginx target
  certree status                  Local connection and target status
  certree logs [-f] [--tail 100]   Operational log (-logs is an alias)
  certree history [--remote]       Local results or central verified history
  certree run                     Run the background worker
  certree service install          Install the Linux systemd worker
  certree gui install              Install an optional loopback web GUI
  certree gui serve                Run the optional GUI in the foreground
  certree version
  certree help
Global option: --state-dir /var/lib/certree-agent
Quit closes the menu only; it does not stop the systemd worker.
`

func CLI(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	dir := "/var/lib/certree-agent"
	for i := 0; i < len(args); i++ {
		if args[i] == "--state-dir" {
			if i+1 >= len(args) {
				return errors.New("--state-dir needs a path")
			}
			dir = args[i+1]
			args = append(args[:i], args[i+2:]...)
			i--
		}
	}
	var e error
	dir, e = filepath.Abs(dir)
	if e != nil {
		return e
	}
	reader := bufio.NewReader(in)
	if len(args) == 0 {
		for {
			fmt.Fprintln(out, "\nCertree Agent\n1. Settings\n2. Status\n3. History\n4. Install Client GUI\n0. Quit")
			choice, err := prompt(reader, out, "Select", "")
			if err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			var command []string
			switch choice {
			case "0":
				return nil
			case "1":
				command = []string{"settings"}
			case "2":
				command = []string{"status"}
			case "3":
				command = []string{"history"}
			case "4":
				command = []string{"gui", "install"}
			default:
				fmt.Fprintln(out, "Choose 0–4.")
				continue
			}
			if err = dispatch(ctx, dir, command, reader, out); err != nil {
				fmt.Fprintln(out, "Error:", err)
			}
		}
	}
	return dispatch(ctx, dir, args, reader, out)
}
func dispatch(ctx context.Context, dir string, args []string, in *bufio.Reader, out io.Writer) error {
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprint(out, help)
		return nil
	case "version", "--version":
		fmt.Fprintln(out, Version)
		return nil
	case "settings", "setup":
		return settings(ctx, dir, in, out)
	case "status":
		v, e := Status(dir)
		if e != nil {
			return e
		}
		return printJSON(out, v)
	case "logs", "--logs", "-logs":
		return showLogs(ctx, dir, args[1:], out)
	case "history":
		if len(args) > 1 {
			if len(args) != 2 || args[1] != "--remote" {
				return errors.New("usage: certree history [--remote]")
			}
			cfg, e := Load(dir)
			if e != nil {
				return e
			}
			c, e := NewClient(cfg)
			if e != nil {
				return e
			}
			var v any
			if e = c.Request(ctx, "GET", "history", nil, &v); e != nil {
				return e
			}
			return printJSON(out, v)
		}
		v, e := LocalHistory(dir)
		if e != nil {
			return e
		}
		fmt.Fprintln(out, "Local execution results; use history --remote for centrally verified outcomes.")
		return printJSON(out, v)
	case "run":
		return Run(ctx, dir)
	case "service":
		if len(args) == 2 && args[1] == "install" {
			return installService(dir, false, out)
		}
	case "gui":
		if len(args) == 2 {
			switch args[1] {
			case "install":
				return installService(dir, true, out)
			case "serve":
				return ServeGUI(ctx, dir, out)
			}
		}
	}
	return errors.New("unknown command; run certree help")
}
func prompt(in *bufio.Reader, out io.Writer, label, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Fprintf(out, "%s [%s] > ", label, defaultValue)
	} else {
		fmt.Fprintf(out, "%s > ", label)
	}
	s, e := in.ReadString('\n')
	if e != nil {
		return "", e
	}
	s = strings.TrimSpace(s)
	if s == "" {
		s = defaultValue
	}
	return s, nil
}
func settings(ctx context.Context, dir string, in *bufio.Reader, out io.Writer) error {
	if e := privateDir(dir); e != nil {
		return e
	}
	unlock, e := acquire(dir)
	if e != nil {
		return e
	}
	defer unlock()
	cfg, e := Load(dir)
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	if cfg.ID == "" {
		if cfg.Server, e = prompt(in, out, "Central server HTTPS address", ""); e != nil {
			return e
		}
		if cfg.CAFile, e = prompt(in, out, "Trusted CA PEM file (blank uses system trust)", ""); e != nil {
			return e
		}
		if cfg.CAFile != "" {
			if cfg.CAFile, e = filepath.Abs(cfg.CAFile); e != nil {
				return e
			}
		}
		name, _ := os.Hostname()
		name, e = prompt(in, out, "Agent name", name)
		if e != nil {
			return e
		}
		c, e := NewClient(cfg)
		if e != nil {
			return e
		}
		fmt.Fprint(out, "One-time registration key (hidden) > ")
		token, e := ReadSecret(in)
		if e != nil {
			return e
		}
		var credentials agentwire.Credentials
		if e = c.Request(ctx, "POST", "register", agentwire.Registration{Token: token, Name: name}, &credentials); e != nil {
			return e
		}
		cfg.ID = credentials.ID
		cfg.Token = credentials.Token
		if e = Save(dir, cfg); e != nil {
			return fmt.Errorf("registered as %s, but credentials could not be saved; revoke this agent in Settings and enroll again: %w", cfg.ID, e)
		}
		fmt.Fprintln(out, "Registered. Credentials saved with restricted permissions.")
	} else {
		fmt.Fprintln(out, "Existing enrollment:", cfg.Server, "·", cfg.ID)
		fmt.Fprintln(out, "Existing targets are immutable. Stop the worker before adding another target.")
	}
	c, e := NewClient(cfg)
	if e != nil {
		return e
	}
	// Reconcile any target saved before an interrupted registration request.
	for _, target := range cfg.Targets {
		if e = c.Request(ctx, "POST", "targets", target, nil); e != nil {
			return e
		}
	}
	var inventory []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if e = c.Request(ctx, "GET", "inventory", nil, &inventory); e != nil {
		return e
	}
	if len(inventory) == 0 {
		fmt.Fprintln(out, "No certificates in the assigned environment. Upload a certificate in the central console, then run certree settings again.")
		return nil
	}
	fmt.Fprintln(out, "\nCertificates in the assigned environment:")
	for i, item := range inventory {
		fmt.Fprintf(out, "%d. %s (%s)\n", i+1, item.Name, item.ID)
	}
	selection, e := prompt(in, out, "Certificate number (0 to finish)", "0")
	if e != nil {
		return e
	}
	n, e := strconv.Atoi(selection)
	if e != nil || n < 0 || n > len(inventory) {
		return errors.New("invalid certificate selection")
	}
	if n == 0 {
		return nil
	}
	fmt.Fprintln(out, "\nService adapter\n1. Nginx (supported)")
	adapter, e := prompt(in, out, "Adapter", "1")
	if e != nil {
		return e
	}
	if adapter != "1" {
		return errors.New("only Nginx is supported in this version")
	}
	t := agentwire.Target{ID: RandomID(), CertificateID: inventory[n-1].ID, Type: "nginx"}
	for _, field := range []struct {
		label, defaultValue string
		value               *string
	}{
		{"Certificate PEM path", "/etc/nginx/ssl/fullchain.pem", &t.CertificatePath},
		{"Private key PEM path", "/etc/nginx/ssl/privkey.pem", &t.KeyPath},
		{"Nginx configuration path", "/etc/nginx/nginx.conf", &t.ConfigurationPath},
		{"TLS verification hostname/IP (reachable from central server)", "", &t.Host},
		{"TLS SNI hostname", "", &t.SNI},
	} {
		if *field.value, e = prompt(in, out, field.label, field.defaultValue); e != nil {
			return e
		}
	}
	port, e := prompt(in, out, "TLS port", "443")
	if e != nil {
		return e
	}
	t.Port, e = strconv.Atoi(port)
	if e != nil || t.Port < 1 || t.Port > 65535 {
		return errors.New("invalid TLS port")
	}
	if e = Check(ctx, t); e != nil {
		return fmt.Errorf("local preflight failed: %w", e)
	}
	for _, old := range cfg.Targets {
		for _, p := range []string{t.CertificatePath, t.KeyPath} {
			if p == old.CertificatePath || p == old.KeyPath || p == old.ConfigurationPath {
				return errors.New("target paths overlap an existing target")
			}
		}
		if t.ConfigurationPath == old.CertificatePath || t.ConfigurationPath == old.KeyPath {
			return errors.New("configuration overlaps another target")
		}
	}
	printJSON(out, t)
	confirm, e := prompt(in, out, "Register this target? (yes/no)", "no")
	if e != nil {
		return e
	}
	if confirm != "yes" {
		return nil
	}
	cfg.Targets = append(cfg.Targets, t)
	if e = Save(dir, cfg); e != nil {
		return e
	}
	if e = c.Request(ctx, "POST", "targets", t, nil); e != nil {
		var rejected *APIError
		if errors.As(e, &rejected) && rejected.Status >= 400 && rejected.Status < 500 {
			cfg.Targets = cfg.Targets[:len(cfg.Targets)-1]
			if saveErr := Save(dir, cfg); saveErr != nil {
				return fmt.Errorf("target rejected (%v), and local draft could not be removed: %w", e, saveErr)
			}
			return fmt.Errorf("target rejected; run settings to correct the configuration: %w", e)
		}
		return fmt.Errorf("saved locally; run settings again to retry registration: %w", e)
	}
	fmt.Fprintln(out, "Target registered; no certificate was replaced.\nRun certree service install, then systemctl enable --now certree-agent.\nFor an existing service, run systemctl restart certree-agent.")
	return nil
}
func printJSON(out io.Writer, v any) error {
	e := json.NewEncoder(out)
	e.SetIndent("", "  ")
	return e.Encode(v)
}
func Status(dir string) (map[string]any, error) {
	c, e := Load(dir)
	if os.IsNotExist(e) {
		return map[string]any{"configured": false, "next": "certree settings"}, nil
	}
	if e != nil {
		return nil, e
	}
	s := map[string]any{"configured": true, "server": c.Server, "agent_id": c.ID, "targets": c.Targets, "connected": false}
	b, e := os.ReadFile(filepath.Join(dir, "status.json"))
	if e == nil {
		var v map[string]any
		if json.Unmarshal(b, &v) == nil {
			s["worker"] = v
			stamp, _ := v["checked_at"].(string)
			t, _ := time.Parse(time.RFC3339Nano, stamp)
			s["connected"] = v["connected"] == true && time.Since(t) < 90*time.Second
			s["stale"] = time.Since(t) >= 90*time.Second
		}
	}
	return s, nil
}
func showLogs(ctx context.Context, dir string, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("logs", flag.ContinueOnError)
	flags.SetOutput(out)
	tail := flags.Int("tail", 100, "last lines")
	follow := flags.Bool("f", false, "follow log")
	if e := flags.Parse(args); e != nil {
		return e
	}
	if flags.NArg() != 0 || *tail < 1 || *tail > 10000 {
		return errors.New("tail must be between 1 and 10000")
	}
	var offset int64
	var previous os.FileInfo
	for {
		p := filepath.Join(dir, "agent.log")
		f, e := os.Open(p)
		if os.IsNotExist(e) {
			if !*follow {
				fmt.Fprintln(out, "No logs yet.")
				return nil
			}
		} else if e != nil {
			return e
		} else {
			st, e := f.Stat()
			if e != nil {
				f.Close()
				return e
			}
			if previous != nil && (!os.SameFile(previous, st) || st.Size() < offset) {
				offset = 0
			}
			if previous == nil {
				b, e := io.ReadAll(io.LimitReader(f, 5<<20))
				if e != nil {
					f.Close()
					return e
				}
				lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
				if len(lines) > *tail {
					lines = lines[len(lines)-*tail:]
				}
				if len(b) > 0 {
					fmt.Fprintln(out, strings.Join(lines, "\n"))
				}
				offset = int64(len(b))
			} else {
				f.Seek(offset, io.SeekStart)
				n, e := io.Copy(out, f)
				if e != nil {
					f.Close()
					return e
				}
				offset += n
			}
			previous = st
			f.Close()
		}
		if !*follow {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
}
