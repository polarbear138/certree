package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	imap "github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
	"io"
	"net"
	"net/http"
	"path"
	"strings"
	"time"
)

type CollectorConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Mailbox  string `json:"mailbox"`
	From     string `json:"from"`
	Subject  string `json:"subject"`
	Interval int    `json:"interval"`
}
type Collector interface {
	Collect(context.Context, CollectorConfig, string, func(string, []byte) error) error
}
type IMAPCollector struct{ TLSConfig *tls.Config }

func (c IMAPCollector) dial(ctx context.Context, cfg CollectorConfig, password string) (*client.Client, func(), error) {
	tc := c.TLSConfig
	if tc == nil {
		tc = &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}
	}
	d := tls.Dialer{NetDialer: &net.Dialer{Timeout: 10 * time.Second}, Config: tc}
	conn, e := d.DialContext(ctx, "tcp", net.JoinHostPort(cfg.Host, fmt.Sprint(cfg.Port)))
	if e != nil {
		return nil, nil, e
	}
	conn.SetDeadline(time.Now().Add(45 * time.Second))
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	cl, e := client.New(conn)
	if e != nil {
		stop()
		conn.Close()
		return nil, nil, e
	}
	cleanup := func() { stop(); cl.Terminate() }
	if e = cl.Login(cfg.Username, password); e != nil {
		cleanup()
		return nil, nil, errors.New("IMAP login failed")
	}
	return cl, cleanup, nil
}
func (c IMAPCollector) Collect(ctx context.Context, cfg CollectorConfig, password string, accept func(string, []byte) error) error {
	cl, close, e := c.dial(ctx, cfg, password)
	if e != nil {
		return e
	}
	defer close()
	mb, e := cl.Select(cfg.Mailbox, true)
	if e != nil {
		return errors.New("cannot select IMAP mailbox")
	}
	criteria := imap.NewSearchCriteria()
	criteria.Since = time.Now().AddDate(0, 0, -30)
	if cfg.From != "" {
		criteria.Header.Add("From", cfg.From)
	}
	if cfg.Subject != "" {
		criteria.Header.Add("Subject", cfg.Subject)
	}
	ids, e := cl.UidSearch(criteria)
	if e != nil {
		return e
	}
	if len(ids) > 100 {
		ids = ids[len(ids)-100:]
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		set := new(imap.SeqSet)
		set.AddNum(id)
		section := &imap.BodySectionName{Peek: true}
		messages := make(chan *imap.Message, 1)
		done := make(chan error, 1)
		go func() {
			done <- cl.UidFetch(set, []imap.FetchItem{imap.FetchUid, imap.FetchRFC822Size, section.FetchItem()}, messages)
		}()
		var data []byte
		for msg := range messages {
			if msg.Size > MaxUpload {
				continue
			}
			body := msg.GetBody(section)
			if body != nil {
				data, e = io.ReadAll(io.LimitReader(body, MaxUpload+1))
				if e != nil {
					break
				}
			}
		}
		if fetchErr := <-done; fetchErr != nil {
			return fetchErr
		}
		if e != nil {
			return e
		}
		key := fmt.Sprintf("%d:%d", mb.UidValidity, id)
		if len(data) > MaxUpload {
			return errors.New("message exceeds 16 MiB")
		}
		if e = accept(key, data); e != nil {
			return e
		}
	}
	return nil
}
func attachments(data []byte) ([][]byte, error) {
	r, e := mail.CreateReader(bytes.NewReader(data))
	if e != nil {
		return nil, e
	}
	var plain [][]byte
	var packed [][]byte
	total := 0
	for count := 0; ; count++ {
		part, e := r.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
		if count >= 64 {
			return nil, errors.New("too many MIME parts")
		}
		h, ok := part.Header.(*mail.AttachmentHeader)
		if !ok {
			continue
		}
		name, _ := h.Filename()
		ext := strings.ToLower(path.Ext(name))
		switch ext {
		case ".pem", ".crt", ".cer", ".key", ".zip", ".pfx", ".p12":
		default:
			continue
		}
		b, e := io.ReadAll(io.LimitReader(part.Body, MaxUpload+1))
		if e != nil {
			return nil, e
		}
		total += len(b)
		if total > MaxUpload {
			return nil, errors.New("attachments exceed 16 MiB")
		}
		if ext == ".zip" || ext == ".pfx" || ext == ".p12" {
			packed = append(packed, b)
		} else {
			plain = append(plain, b)
		}
	}
	if len(plain) > 0 {
		var buf bytes.Buffer
		z := zip.NewWriter(&buf)
		for i, b := range plain {
			f, _ := z.Create(fmt.Sprintf("attachment-%d", i))
			f.Write(b)
		}
		z.Close()
		packed = append(packed, buf.Bytes())
	}
	if len(packed) == 0 {
		return nil, errors.New("no supported attachments")
	}
	return packed, nil
}
func (a *App) collector(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name          string `json:"name"`
		EnvironmentID string `json:"environment_id"`
		Password      string `json:"password"`
		CollectorConfig
	}
	if !decode(w, r, &b) {
		return
	}
	if b.Name == "" || !validHost(b.Host) || b.Port < 1 || b.Port > 65535 || b.Username == "" || b.Password == "" || len(b.Mailbox) > 120 || b.Interval < 60 || b.Interval > 86400 {
		problem(w, 400, "collector requires name, TLS IMAP host, credentials and interval 60–86400 seconds")
		return
	}
	if b.Mailbox == "" {
		b.Mailbox = "INBOX"
	}
	id := ID()
	if _, e := a.Store.DB.Exec("INSERT INTO collectors(id,name,environment_id,config,secret) VALUES(?,?,?,?,?)", id, b.Name, b.EnvironmentID, encode(b.CollectorConfig), a.Store.Seal([]byte(b.Password))); e != nil {
		problem(w, 400, "valid environment required")
		return
	}
	a.Store.Audit(actor(r), "collector_created", id)
	response(w, 201, map[string]string{"id": id})
}
func (a *App) poll(w http.ResponseWriter, r *http.Request) {
	res, e := a.Store.DB.Exec("UPDATE collectors SET next_poll=0 WHERE id=?", r.PathValue("id"))
	if e != nil {
		problem(w, 500, "queue failed")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		problem(w, 404, "collector not found")
		return
	}
	a.Store.Audit(actor(r), "collector_poll_requested", r.PathValue("id"))
	response(w, 202, map[string]string{"status": "Pending"})
}
func (a *App) testCollector(w http.ResponseWriter, r *http.Request) {
	var conf string
	var enc []byte
	if e := a.Store.DB.QueryRow("SELECT config,secret FROM collectors WHERE id=?", r.PathValue("id")).Scan(&conf, &enc); e != nil {
		problem(w, 404, "collector not found")
		return
	}
	var c CollectorConfig
	json.Unmarshal([]byte(conf), &c)
	password, e := a.Store.Unseal(enc)
	if e != nil {
		problem(w, 500, "secret unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	cl, close, e := (IMAPCollector{&tls.Config{ServerName: c.Host, RootCAs: a.Roots, MinVersion: tls.VersionTLS12}}).dial(ctx, c, string(password))
	if e != nil {
		problem(w, 422, "IMAP connection or authentication failed")
		return
	}
	defer close()
	if _, e = cl.Select(c.Mailbox, true); e != nil {
		problem(w, 422, "mailbox unavailable")
		return
	}
	a.Store.Audit(actor(r), "collector_connection_test", r.PathValue("id"))
	response(w, 200, map[string]bool{"connected": true})
}
func (a *App) PollDue(ctx context.Context) {
	a.collectorMu.Lock()
	defer a.collectorMu.Unlock()
	rows, e := a.Store.Rows("SELECT id,environment_id,config,attempt FROM collectors WHERE next_poll<=?", time.Now().Unix())
	if e != nil {
		return
	}
	for _, row := range rows {
		id := row["id"].(string)
		var c CollectorConfig
		json.Unmarshal([]byte(row["config"].(string)), &c)
		var enc []byte
		a.Store.DB.QueryRow("SELECT secret FROM collectors WHERE id=?", id).Scan(&enc)
		password, e := a.Store.Unseal(enc)
		if e != nil {
			continue
		}
		pollctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		e = (IMAPCollector{&tls.Config{ServerName: c.Host, RootCAs: a.Roots, MinVersion: tls.VersionTLS12}}).Collect(pollctx, c, string(password), func(key string, data []byte) error {
			var seen int
			a.Store.DB.QueryRow("SELECT count(*) FROM collector_events WHERE collector_id=? AND message_key=?", id, key).Scan(&seen)
			if seen > 0 {
				return nil
			}
			bundles, err := attachments(data)
			result := "Imported; administrator approval required"
			if err == nil {
				for _, bundle := range bundles {
					parsed, parseErr := ParseBundle(bundle, "")
					if parseErr != nil {
						err = parseErr
						break
					}
					if !parsed.Validate(a.Roots, "").Valid {
						err = errors.New("certificate trust, lifetime or key validation failed; manual inspection required")
						break
					}
					if _, err = a.Store.Import(row["environment_id"].(string), bundle, "", a.Roots); err != nil {
						break
					}
				}
			}
			if err != nil {
				result = "Rejected: " + err.Error()
				if len(result) > 512 {
					result = result[:512] + "…"
				}
			}
			_, err = a.Store.DB.Exec("INSERT OR IGNORE INTO collector_events VALUES(?,?,?,?,?)", ID(), id, key, result, time.Now().UTC().Format(timestampFormat))
			return err
		})
		cancel()
		attempt := int(row["attempt"].(int64))
		message := ""
		delay := c.Interval
		if e != nil {
			message = "IMAP polling failed; check connection, credentials, mailbox or attachment limits"
			attempt++
			delay = 60 * (1 << min(attempt, 6))
			if attempt >= 5 {
				delay = 86400
			}
		} else {
			attempt = 0
		}
		a.Store.DB.Exec("UPDATE collectors SET last_poll=?,last_error=?,next_poll=?,attempt=? WHERE id=?", time.Now().Unix(), message, time.Now().Add(time.Duration(delay)*time.Second).Unix(), attempt, id)
		a.Store.Audit("worker", "collector_polled", id)
	}
}
