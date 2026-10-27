//go:build windows

package kuberneteshelm

import (
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsRenderLimiter windows.Handle

func newRenderLimiter() (renderLimiter, error) {
	job, e := windows.CreateJobObject(nil, nil)
	if e != nil {
		return nil, e
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY | windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS | windows.JOB_OBJECT_LIMIT_PROCESS_TIME
	limits.BasicLimitInformation.ActiveProcessLimit = 1
	limits.BasicLimitInformation.PerProcessUserTimeLimit = 5 * 10_000_000
	limits.ProcessMemoryLimit = 128 << 20
	if _, e = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); e != nil {
		windows.CloseHandle(job)
		return nil, e
	}
	return windowsRenderLimiter(job), nil
}
func (l windowsRenderLimiter) Attach(p *os.Process) error {
	process, e := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_INFORMATION, false, uint32(p.Pid))
	if e != nil {
		return e
	}
	defer windows.CloseHandle(process)
	return windows.AssignProcessToJobObject(windows.Handle(l), process)
}
func (l windowsRenderLimiter) Close() error { return windows.CloseHandle(windows.Handle(l)) }
func configureRenderCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
func limitRenderSelf() error { return nil } // Job is assigned before the parent writes stdin.
