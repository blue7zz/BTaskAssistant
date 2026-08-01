//go:build windows

package agent

import (
	"os"
	"os/exec"
)

func configureProcessGroup(_ *exec.Cmd) {}

func terminateProcess(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return nil
	}
	return command.Process.Signal(os.Interrupt)
}

func killProcess(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return nil
	}
	return command.Process.Kill()
}

func processSignal(_ *os.ProcessState) string { return "" }
