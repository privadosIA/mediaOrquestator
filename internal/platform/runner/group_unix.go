//go:build unix

package runner

import (
	"os/exec"
	"syscall"
)

// inOwnGroup makes the program the leader of a new process group and makes cancelling its context kill the whole group,
// so a program that starts others (Blender, dotnet, a shell) leaves none of them behind and the pipes it shared with them
// close.
func inOwnGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
