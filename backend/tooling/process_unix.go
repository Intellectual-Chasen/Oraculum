//go:build unix

package tooling

import (
	"errors"
	"os/exec"
	"syscall"
)

// isolateProcessGroup は子 process を新しい process group に置く。止めるときに、子が起動した
// process までまとめて止める。
func isolateProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup は子 process の process group に SIGKILL を送る。
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
