package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"time"
)

type Store struct {
	DB    *sql.DB
	box   cipher.AEAD
	lease *os.File
}

// Fixed fractional precision keeps SQLite TEXT timestamp ordering chronological.
const timestampFormat = "2006-01-02T15:04:05.000000000Z"

func ID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func Open(path, key string) (*Store, error) {
	k, e := base64.StdEncoding.DecodeString(key)
	if e != nil || len(k) != 32 {
		return nil, fmt.Errorf("CERTREE_MASTER_KEY must be base64 of 32 random bytes")
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return nil, e
	}
	lease, e := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = lockDatabase(lease); e != nil {
		lease.Close()
		return nil, fmt.Errorf("database already owned by another Certree process")
	}
	opened := false
	defer func() {
		if !opened {
			lease.Close()
		}
	}()
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	b, _ := aes.NewCipher(k)
	box, _ := cipher.NewGCM(b)
	s := &Store{DB: db, box: box, lease: lease}
	_, e = db.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;")
	if e != nil {
		db.Close()
		return nil, e
	}
	tx, e := db.Begin()
	if e != nil {
		db.Close()
		return nil, e
	}
	_, e = tx.Exec(schema + agentSchema)
	if e == nil {
		e = tx.Commit()
	} else {
		tx.Rollback()
	}
	if e != nil {
		db.Close()
		return nil, e
	}
	if e = os.Chmod(path, 0600); e != nil {
		db.Close()
		return nil, e
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if e = os.Chmod(path+suffix, 0600); e != nil && !os.IsNotExist(e) {
			db.Close()
			return nil, e
		}
	}
	var sample []byte
	if e = db.QueryRow("SELECT secret FROM certificate_secrets UNION ALL SELECT secret FROM servers UNION ALL SELECT secret FROM collectors LIMIT 1").Scan(&sample); e == nil {
		if _, e = s.Unseal(sample); e != nil {
			db.Close()
			return nil, fmt.Errorf("master key cannot decrypt existing data")
		}
	}
	opened = true
	return s, nil
}
func (s *Store) Close() error {
	e := s.DB.Close()
	if s.lease != nil {
		if closeErr := s.lease.Close(); e == nil {
			e = closeErr
		}
		s.lease = nil
	}
	return e
}
func (s *Store) Seal(b []byte) []byte {
	n := make([]byte, s.box.NonceSize())
	if _, e := rand.Read(n); e != nil {
		panic(e)
	}
	return s.box.Seal(n, n, b, []byte("certree-v1"))
}
func (s *Store) Unseal(b []byte) ([]byte, error) {
	n := s.box.NonceSize()
	if len(b) < n {
		return nil, fmt.Errorf("invalid secret")
	}
	return s.box.Open(nil, b[:n], b[n:], []byte("certree-v1"))
}
func (s *Store) Audit(actor, action, entity string) {
	s.DB.Exec("INSERT INTO audit_logs(id,actor,action,entity,created_at) VALUES(?,?,?,?,?)", ID(), actor, action, entity, time.Now().UTC().Format(time.RFC3339))
}
func (s *Store) Rows(query string, args ...any) ([]map[string]any, error) {
	rows, e := s.DB.Query(query, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptr := make([]any, len(cols))
		for i := range vals {
			ptr[i] = &vals[i]
		}
		if e = rows.Scan(ptr...); e != nil {
			return nil, e
		}
		m := map[string]any{}
		for i, c := range cols {
			v := vals[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			m[c] = v
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func encode(v any) string { b, _ := json.Marshal(v); return string(b) }

const schema = `
CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY);
CREATE TABLE IF NOT EXISTS users(id TEXT PRIMARY KEY,username TEXT UNIQUE NOT NULL,password BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS sessions(token TEXT PRIMARY KEY,user_id TEXT REFERENCES users(id),csrf TEXT NOT NULL,expires INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS organizations(id TEXT PRIMARY KEY,name TEXT UNIQUE NOT NULL);
CREATE TABLE IF NOT EXISTS environments(id TEXT PRIMARY KEY,organization_id TEXT NOT NULL REFERENCES organizations(id),name TEXT NOT NULL,UNIQUE(organization_id,name));
CREATE TABLE IF NOT EXISTS certificates(id TEXT PRIMARY KEY,name TEXT NOT NULL,environment_id TEXT NOT NULL REFERENCES environments(id),UNIQUE(environment_id,name));
CREATE TABLE IF NOT EXISTS certificate_versions(id TEXT PRIMARY KEY,certificate_id TEXT NOT NULL REFERENCES certificates(id),fingerprint TEXT NOT NULL,metadata TEXT NOT NULL,pem TEXT NOT NULL,validation TEXT NOT NULL,approved INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL,UNIQUE(certificate_id,fingerprint));
CREATE TABLE IF NOT EXISTS certificate_secrets(version_id TEXT PRIMARY KEY REFERENCES certificate_versions(id),secret BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS certificate_chains(version_id TEXT PRIMARY KEY REFERENCES certificate_versions(id),pem TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS servers(id TEXT PRIMARY KEY,name TEXT NOT NULL,config TEXT NOT NULL,secret BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS server_endpoints(id TEXT PRIMARY KEY,server_id TEXT NOT NULL REFERENCES servers(id),host TEXT NOT NULL,port INTEGER NOT NULL,sni TEXT NOT NULL,UNIQUE(server_id,host,port,sni));
CREATE TABLE IF NOT EXISTS deployment_targets(id TEXT PRIMARY KEY,endpoint_id TEXT NOT NULL REFERENCES server_endpoints(id),certificate_id TEXT NOT NULL REFERENCES certificates(id),config TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'Unknown',deployment_status TEXT NOT NULL DEFAULT 'Unknown',verification_status TEXT NOT NULL DEFAULT 'Unknown',active_version TEXT,UNIQUE(endpoint_id,certificate_id));
CREATE TABLE IF NOT EXISTS deployment_jobs(id TEXT PRIMARY KEY,target_id TEXT NOT NULL REFERENCES deployment_targets(id),server_id TEXT NOT NULL REFERENCES servers(id),version_id TEXT NOT NULL REFERENCES certificate_versions(id),kind TEXT NOT NULL,status TEXT NOT NULL,stage TEXT NOT NULL DEFAULT 'Queued',error TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL,updated_at TEXT NOT NULL,attempt INTEGER NOT NULL DEFAULT 0,next_run INTEGER NOT NULL DEFAULT 0);
DROP INDEX IF EXISTS active_target_job;
CREATE UNIQUE INDEX active_target_job ON deployment_jobs(target_id) WHERE status IN ('Queued','Pending','Running','Recovering');
CREATE UNIQUE INDEX IF NOT EXISTS active_server_job ON deployment_jobs(server_id) WHERE status IN ('Pending','Running','Recovering');
CREATE TABLE IF NOT EXISTS deployment_results(id TEXT PRIMARY KEY,job_id TEXT REFERENCES deployment_jobs(id),outcome TEXT NOT NULL,detail TEXT NOT NULL,created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS verification_results(id TEXT PRIMARY KEY,target_id TEXT REFERENCES deployment_targets(id),job_id TEXT REFERENCES deployment_jobs(id),expected TEXT NOT NULL,actual TEXT NOT NULL,result TEXT NOT NULL,created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS collectors(id TEXT PRIMARY KEY,name TEXT NOT NULL,environment_id TEXT NOT NULL REFERENCES environments(id),config TEXT NOT NULL,secret BLOB NOT NULL,last_poll INTEGER NOT NULL DEFAULT 0,last_error TEXT NOT NULL DEFAULT '',next_poll INTEGER NOT NULL DEFAULT 0,attempt INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS collector_events(id TEXT PRIMARY KEY,collector_id TEXT REFERENCES collectors(id),message_key TEXT NOT NULL,result TEXT NOT NULL,created_at TEXT NOT NULL,UNIQUE(collector_id,message_key));
CREATE TABLE IF NOT EXISTS audit_logs(id TEXT PRIMARY KEY,actor TEXT NOT NULL,action TEXT NOT NULL,entity TEXT NOT NULL,created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS settings(key TEXT PRIMARY KEY,value TEXT NOT NULL);
INSERT OR IGNORE INTO settings VALUES('expiry_days','30');
INSERT OR IGNORE INTO schema_migrations VALUES(1);`
