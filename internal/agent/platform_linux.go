package agent

import (
	"bufio"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func trustedOwner(st os.FileInfo) bool {
	s, ok := st.Sys().(*syscall.Stat_t)
	return ok && (s.Uid == 0 || s.Uid == uint32(os.Geteuid()))
}

func acquire(dir string) (func(), error) {
	p := filepath.Join(dir, "agent.lock")
	if e := safePath(p, false); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, fmt.Errorf("agent is already running; stop certree-agent before changing settings")
	}
	return func() { f.Close() }, nil
}
func ReadSecret(reader *bufio.Reader) (string, error) {
	fd := int(os.Stdin.Fd())
	old, e := unix.IoctlGetTermios(fd, unix.TCGETS)
	if e == nil {
		next := *old
		next.Lflag &^= unix.ECHO
		if e = unix.IoctlSetTermios(fd, unix.TCSETS, &next); e != nil {
			return "", e
		}
		defer unix.IoctlSetTermios(fd, unix.TCSETS, old)
	}
	s, e := reader.ReadString('\n')
	fmt.Println()
	return strings.TrimSpace(s), e
}
