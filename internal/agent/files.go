package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Paths are locally configured, never accepted from a deployment task.
func safePath(p string, required bool) error {
	if !filepath.IsAbs(p) || filepath.Clean(p) != p {
		return errors.New("clean absolute path required")
	}
	for current := p; ; current = filepath.Dir(current) {
		st, e := os.Lstat(current)
		if e != nil {
			if os.IsNotExist(e) && current == p && !required {
				continue
			}
			return e
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not allowed: %s", current)
		}
		if !trustedOwner(st) {
			return fmt.Errorf("path must be owned by root or the agent user: %s", current)
		}
		if current != p && !st.IsDir() {
			return errors.New("parent must be a directory")
		}
		if runtime.GOOS != "windows" && st.IsDir() && st.Mode().Perm()&0022 != 0 && st.Mode()&os.ModeSticky == 0 {
			return fmt.Errorf("directory is writable by other users: %s", current)
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	return nil
}
func privateDir(p string) error {
	if !filepath.IsAbs(p) {
		return errors.New("absolute state directory required")
	}
	var created []string
	for current := p; ; current = filepath.Dir(current) {
		_, err := os.Lstat(current)
		if err == nil {
			if err = safePath(current, true); err != nil {
				return err
			}
			break
		}
		if !os.IsNotExist(err) {
			return err
		}
		created = append(created, current)
		if filepath.Dir(current) == current {
			return err
		}
	}
	if e := os.MkdirAll(p, 0700); e != nil {
		return e
	}
	if e := safePath(p, true); e != nil {
		return e
	}
	st, e := os.Stat(p)
	if e != nil {
		return e
	}
	if !st.IsDir() {
		return errors.New("state must be a directory")
	}
	if runtime.GOOS != "windows" && st.Mode().Perm()&0077 != 0 {
		return errors.New("state directory must have mode 0700")
	}
	// Persist newly created job-directory links before replacing managed files.
	for _, current := range created {
		if e := syncDirectory(current); e != nil {
			return e
		}
		if e := syncDirectory(filepath.Dir(current)); e != nil {
			return e
		}
	}
	return nil
}
func atomicWrite(p string, b []byte, mode os.FileMode) error {
	if e := safePath(p, false); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(p), ".certree-")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(tmp, p); e != nil {
		return e
	}
	return syncDirectory(filepath.Dir(p))
}
func syncDirectory(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	f, e := os.Open(dir)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
