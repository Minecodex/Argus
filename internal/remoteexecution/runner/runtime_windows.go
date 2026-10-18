package runner

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

type powershellRuntime struct {
	cmd           *exec.Cmd
	conn          net.Conn
	reader        *bufio.Reader
	root, version string
	job           windows.Handle
	once          sync.Once
	done          chan struct{}
	closeErr      error
}

// A private control socket carries completion receipts. PowerShell stdout and
// stderr are data fields and cannot be interpreted as protocol frames.
const powershellSupervisor = `$ErrorActionPreference='Stop'
$__argusClient=[Net.Sockets.TcpClient]::new('127.0.0.1',PORT)
$__argusStream=$__argusClient.GetStream()
$__argusReader=[IO.StreamReader]::new($__argusStream,[Text.UTF8Encoding]::new($false))
$__argusWriter=[IO.StreamWriter]::new($__argusStream,[Text.UTF8Encoding]::new($false));$__argusWriter.AutoFlush=$true
$__argusWriter.WriteLine(('TOKEN' + '|' + $PSVersionTable.PSVersion.ToString()))
$__argusRunspace=[Management.Automation.Runspaces.RunspaceFactory]::CreateRunspace();$__argusRunspace.Open()
$__argusFormatter=Get-Command -CommandType Cmdlet Microsoft.PowerShell.Utility\Out-String
while (($__argusLine=$__argusReader.ReadLine()) -ne $null) {
  $__argusRequest=ConvertFrom-Json -InputObject $__argusLine
  $__argusExit=0
  $env:ARGUS_INPUT_DIR=$__argusRequest.input
  $env:ARGUS_OUTPUT_DIR=$__argusRequest.output
  $__argusRunspace.SessionStateProxy.SetVariable('ErrorActionPreference','Stop')
  $__argusRunspace.SessionStateProxy.SetVariable('LASTEXITCODE',0)
  $__argusPipeline=[Management.Automation.PowerShell]::Create();$__argusPipeline.Runspace=$__argusRunspace
  $__argusText='';$__argusErrorText=''
  try {
    $null=$__argusPipeline.AddScript($__argusRequest.script,$false).AddCommand($__argusFormatter)
    $__argusValues=$__argusPipeline.Invoke()
    foreach ($__argusValue in $__argusValues) { if ($__argusValue -is [string]) { $__argusText+=[string]$__argusValue } }
    if ($__argusPipeline.HadErrors) { $__argusExit=1 }
    $__argusNativeExit=$__argusRunspace.SessionStateProxy.GetVariable('LASTEXITCODE')
    if ($__argusNativeExit -ne 0) { $__argusExit=[int]$__argusNativeExit }
    foreach ($__argusError in $__argusPipeline.Streams.Error) { $__argusErrorText+=($__argusError.ToString()+[Environment]::NewLine) }
    foreach ($__argusInfo in $__argusPipeline.Streams.Information) { $__argusText+=($__argusInfo.MessageData.ToString()+[Environment]::NewLine) }
  } catch {
    $__argusExit=1
    $__argusErrorText+=($_ | Out-String)
  } finally { $__argusPipeline.Dispose() }
  $__argusBudget=8388608;$__argusTruncated=$false;$__argusEncoded=@()
  foreach ($__argusCapture in @(@{text=$__argusText},@{text=$__argusErrorText})) {
    $__argusBytes=[Text.Encoding]::UTF8.GetBytes($__argusCapture.text)
    if ($__argusBytes.Length -gt $__argusBudget) { $__argusTruncated=$true;$__argusBounded=[byte[]]::new($__argusBudget);[Array]::Copy($__argusBytes,$__argusBounded,$__argusBudget);$__argusBytes=$__argusBounded }
    $__argusEncoded+=,[Convert]::ToBase64String($__argusBytes)
  }
  $__argusWriter.WriteLine((ConvertTo-Json -Compress -InputObject @{nonce=$__argusRequest.nonce;exit_code=$__argusExit;directory=$__argusRunspace.SessionStateProxy.Path.CurrentLocation.Path;stdout=$__argusEncoded[0];stderr=$__argusEncoded[1];truncated=$__argusTruncated}))
}
$__argusRunspace.Dispose()
`

func encodePowerShell(v string) string {
	words := utf16.Encode([]rune(v))
	b := make([]byte, len(words)*2)
	for i, w := range words {
		binary.LittleEndian.PutUint16(b[i*2:], w)
	}
	return base64.StdEncoding.EncodeToString(b)
}
func newRuntime(ctx context.Context, root, interpreter string, kubernetes bool) (commandRuntime, error) {
	if interpreter != "powershell" {
		return nil, errors.New("REMOTE_INTERPRETER_UNAVAILABLE")
	}
	executable := os.Getenv("ARGUS_REMOTE_POWERSHELL_PATH")
	if executable == "" {
		executable = filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer listener.Close()
	token := uuid.NewString() + uuid.NewString()
	script := strings.ReplaceAll(strings.ReplaceAll(powershellSupervisor, "PORT", fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)), "TOKEN", token)
	cmd := exec.Command(executable, "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePowerShell(script))
	cmd.Env = commandEnvironment(kubernetes)
	cmd.Dir = root
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY | windows.JOB_OBJECT_LIMIT_JOB_MEMORY | windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS
	limits.ProcessMemoryLimit = 256 << 20
	limits.JobMemoryLimit = 512 << 20
	limits.BasicLimitInformation.ActiveProcessLimit = 64
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_INFORMATION, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = cmd.Process.Kill()
		windows.CloseHandle(job)
		return nil, err
	}
	err = windows.AssignProcessToJobObject(job, process)
	windows.CloseHandle(process)
	if err != nil {
		_ = cmd.Process.Kill()
		windows.CloseHandle(job)
		return nil, err
	}
	runtime := &powershellRuntime{cmd: cmd, job: job, root: root, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(runtime.done) }()
	_ = listener.(*net.TCPListener).SetDeadline(time.Now().Add(20 * time.Second))
	conn, err := listener.Accept()
	if err != nil {
		_ = runtime.Close()
		return nil, err
	}
	runtime.conn = conn
	runtime.reader = bufio.NewReaderSize(conn, 64<<10)
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	greeting, err := runtime.reader.ReadString('\n')
	_ = conn.SetReadDeadline(time.Time{})
	greeting = strings.TrimSpace(greeting)
	if err != nil || !strings.HasPrefix(greeting, token+"|") {
		_ = runtime.Close()
		return nil, errors.New("REMOTE_CONTROL_AUTHENTICATION_FAILED")
	}
	runtime.version = strings.TrimPrefix(greeting, token+"|")
	if err = ctx.Err(); err != nil {
		_ = runtime.Close()
		return nil, err
	}
	return runtime, nil
}
func (p *powershellRuntime) Version() string { return "PowerShell " + p.version }
func (p *powershellRuntime) Run(ctx context.Context, script, input, output string) (runtimeResult, error) {
	nonce := uuid.NewString()
	request, _ := json.Marshal(map[string]string{"nonce": nonce, "script": script, "input": input, "output": output})
	if deadline, ok := ctx.Deadline(); ok {
		_ = p.conn.SetDeadline(deadline)
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = p.Close()
		case <-done:
		}
	}()
	if _, err := p.conn.Write(append(request, '\n')); err != nil {
		return runtimeResult{ExitCode: -1}, err
	}
	line, err := readBoundedFrame(p.reader, '\n', 24<<20)
	if err != nil {
		return runtimeResult{ExitCode: -1}, err
	}
	var result struct {
		Nonce     string `json:"nonce"`
		ExitCode  int    `json:"exit_code"`
		Directory string `json:"directory"`
		Stdout    []byte `json:"stdout"`
		Stderr    []byte `json:"stderr"`
		Truncated bool   `json:"truncated"`
	}
	if json.Unmarshal(line, &result) != nil || result.Nonce != nonce {
		return runtimeResult{ExitCode: -1}, errors.New("REMOTE_COMPLETION_INVALID")
	}
	var accounting jobAccounting
	if err = windows.QueryInformationJobObject(p.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil); err != nil {
		return runtimeResult{ExitCode: result.ExitCode}, err
	}
	if accounting.ActiveProcesses > 1 {
		return runtimeResult{ExitCode: result.ExitCode, Stdout: result.Stdout, Stderr: result.Stderr, Truncated: result.Truncated}, errors.New("REMOTE_OUTPUT_STILL_ACTIVE")
	}
	return runtimeResult{ExitCode: result.ExitCode, Directory: result.Directory, Stdout: result.Stdout, Stderr: result.Stderr, Truncated: result.Truncated}, nil
}
func (p *powershellRuntime) Close() error {
	p.once.Do(func() {
		if p.conn != nil {
			_ = p.conn.Close()
		}
		if p.job != 0 {
			p.closeErr = windows.TerminateJobObject(p.job, 1)
			if p.closeErr == nil {
				p.closeErr = p.waitJobEmpty(5 * time.Second)
			}
			_ = windows.CloseHandle(p.job)
		}
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
		select {
		case <-p.done:
		case <-time.After(5 * time.Second):
			p.closeErr = errors.New("REMOTE_STOP_UNKNOWN")
		}
	})
	return p.closeErr
}

// Layout follows JOBOBJECT_BASIC_ACCOUNTING_INFORMATION (winnt.h).
type jobAccounting struct {
	TotalUser, TotalKernel, PeriodUser, PeriodKernel                 int64
	PageFaults, TotalProcesses, ActiveProcesses, TerminatedProcesses uint32
}

func (p *powershellRuntime) waitJobEmpty(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		var info jobAccounting
		if err := windows.QueryInformationJobObject(p.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil); err != nil {
			return err
		}
		if info.ActiveProcesses == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("REMOTE_STOP_UNKNOWN")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func isLinkOrReparse(p string, info os.FileInfo) bool {
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	name, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return true
	}
	attributes, err := windows.GetFileAttributes(name)
	return err != nil || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
