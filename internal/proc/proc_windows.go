//go:build windows

package proc

import (
	"context"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const createNoWindow = 0x08000000

// jobObject is a per-process Windows Job Object with KILL_ON_JOB_CLOSE. Every
// CLI subprocess is assigned to it, so when LocalAIProxy.exe exits—normally,
// force-killed, or crashed—the OS terminates the entire tree. This is the real
// fix for orphaned CLI processes (and the consequent stuck "requests running"
// guard): children can never outlive the app once the app process dies.
var (
	jobOnce sync.Once
	job     windows.Handle
	jobErr  error
)

// assignJob joins a spawned process to the app-wide job object so the whole
// tree is killed when the app exits. Best effort: on failure the per-request
// tree killer (ctx) still covers normal cancellation; job-only pieces rely on
// the caller's existing cleanup.
func assignJob() {
	jobOnce.Do(func() {
		h, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			jobErr = err
			return
		}
		var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		_, err = windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
		if err != nil {
			_ = windows.CloseHandle(h)
			jobErr = err
			return
		}
		job = h
	})
}

// joinJob assigns a freshly started process to the job object if one is
// availableaine. Failures are deliberately ignored: the ctx tree killer is the
// backstop for normal cancellation.
func joinJob(pid int) {
	if job == 0 && jobErr == nil {
		return
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	_ = windows.AssignProcessToJobObject(job, handle)
}

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
