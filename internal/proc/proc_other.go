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
	}()
}
