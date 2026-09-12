//go:build windows

package proc

import (
	"context"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

const createNoWindow = 0x08000000

func hideWindowAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

// treeKiller kills the child process tree when ctx is done. Each request has
// its own context that is cancelled when the handler returns, so the watcher
// goroutine never outlives the request.
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
		// If the process already exited, ProcessState != nil; skip the kill.
		if t.cmd.Process != nil && t.cmd.ProcessState == nil {
			killTree(t.cmd.Process.Pid)
		}
	}()
}

// killTree terminates a process and all of its descendants on Windows.
func killTree(pid int) {
	// taskkill /T /F kills the target plus every child. exec.CommandContext
	// already terminates the direct process; this covers .cmd wrappers and
	// grandchildren. Errors are ignored: best effort cleanup.
	taskkill := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
	_ = taskkill.Run()
	// Brief pause so Wait() observes the kill.
	time.Sleep(50 * time.Millisecond)
}
