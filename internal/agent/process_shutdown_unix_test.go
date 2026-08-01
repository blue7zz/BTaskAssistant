//go:build !windows

package agent

import (
	"context"
	"testing"
	"time"
)

func TestProcessCloseEscalatesFromEOFTerminateToKill(t *testing.T) {
	root := t.TempDir()
	options := helperProcessOptions(root)
	options.ShutdownGrace = 30 * time.Millisecond
	options.AdditionalEnv = append(options.AdditionalEnv, "BTASK_PI_HANG_AFTER_EOF=1")
	process, _, err := StartProcess(context.Background(), options)
	if err != nil {
		t.Fatalf("start hanging helper pi: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := process.Close(ctx); err != nil {
		t.Fatalf("close hanging helper pi: %v", err)
	}
	exit := process.Exit()
	if exit.Signal == "" || exit.Code == 0 {
		t.Fatalf("helper was not killed after ignoring graceful shutdown: %#v", exit)
	}
}
