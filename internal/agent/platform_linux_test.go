package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxLockAndSymlinkPolicy(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	unlock, e := acquire(dir)
	if e != nil {
		t.Fatal(e)
	}
	if release, e := acquire(dir); e == nil {
		release()
		t.Fatal("second worker acquired lock")
	}
	unlock()
	unlock, e = acquire(dir)
	if e != nil {
		t.Fatal("lock not released", e)
	}
	unlock()
	if e = os.Symlink(dir, filepath.Join(dir, "link")); e != nil {
		t.Fatal(e)
	}
	if safePath(filepath.Join(dir, "link"), true) == nil {
		t.Fatal("symlink accepted")
	}
}
