//go:build !windows

package agent

import (
	"os/signal"
	"syscall"
)

func configureHelperHang() {
	signal.Ignore(syscall.SIGTERM)
}
