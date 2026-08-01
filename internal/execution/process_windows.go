//go:build windows

package execution

import (
	"errors"
	"os/exec"
	"strconv"
	"syscall"
)

func configureProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func terminateProcessTree(command *exec.Cmd, _ bool) error {
	if command == nil || command.Process == nil {
		return errors.New("Shell process is not running")
	}
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(command.Process.Pid))
	return kill.Run()
}
