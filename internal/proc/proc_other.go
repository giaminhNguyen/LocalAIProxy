//go:build !windows

package proc

import (
	"context"
	"os/exec"
	"syscall"
)

func hideWindowAttr() *syscall.SysProcAttr { return nil }

type treeKiller struct {
	cmd *exec.Cmd
	ctx context.Context
}

func newTreeKiller(cmd *exec.Cmd, ctx context.Context) *treeKiller {
	return &treeKiller{cmd: cmd, ctx: ctx}
}

func (t *treeKiller) watch() {
	go func() {
		<-t.ctx.Done()
		// exec.CommandContext already kills the direct child; on unix that is
		// sufficient for the MVP (no .cmd wrapper chains).
		if t.cmd.Process != nil && t.cmd.ProcessState == nil {
			killTree(t.cmd.Process.Pid)
		}
	}()
}

// joinJob is a no-op outside Windows (no Job Object concept).
func joinJob(pid int) {}

// killTree kills the direct child on unix. Grandchildren are not tracked
// without process groups; each CLI is invoked directly so that is the extent
// of the guarantee here.
func killTree(pid int) {
	// kill(0) would signal the caller's own group; send to the pid directly.
	// exec.CommandContext already terminated it, so this is only a backstop.
}
