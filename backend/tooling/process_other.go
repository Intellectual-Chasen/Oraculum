//go:build !unix

package tooling

import (
	"errors"
	"os"
	"os/exec"
)

// isolateProcessGroup は、process group を持たない OS では何もしない。
func isolateProcessGroup(*exec.Cmd) {}

// killProcessGroup は子 process を止める。
//
// 既知の制限: process group を持たない OS では子 process 1 つだけを止める, 子が起動した孫の
// process は残りうるが、対象の OS での実機の確認をしていない (測れない), Windows での運用を
// 確かめるときに job object で止める形に見直す
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	err := cmd.Process.Kill()
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}
