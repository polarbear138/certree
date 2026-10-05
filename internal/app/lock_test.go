package app

import (
	"bytes"
	"encoding/base64"
	"path/filepath"
	"testing"
)

func TestProcessOwnershipAndRelease(t *testing.T) {
	file := filepath.Join(t.TempDir(), "locked.db")
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32))
	first, e := Open(file, key)
	if e != nil {
		t.Fatal(e)
	}
	if second, e := Open(file, key); e == nil {
		second.Close()
		first.Close()
		t.Fatal("second process could own database")
	}
	first.Close()
	second, e := Open(file, key)
	if e != nil {
		t.Fatal("lock did not release", e)
	}
	second.Close()
}
