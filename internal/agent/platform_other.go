//go:build !linux

package agent

import (
	"bufio"
	"errors"
	"os"
)

func trustedOwner(os.FileInfo) bool { return true }

func acquire(string) (func(), error) {
	return nil, errors.New("agent service and settings are supported on Linux only")
}
func ReadSecret(*bufio.Reader) (string, error) {
	return "", errors.New("interactive enrollment is supported on Linux only")
}
