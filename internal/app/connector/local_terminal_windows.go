//go:build windows

package connector

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsConPTY struct {
	input       *os.File
	output      *os.File
	inputRead   windows.Handle
	outputWrite windows.Handle
	console     windows.Handle
	process     windows.Handle
	closeOnce   sync.Once
}

func openLocalTerminal(ctx context.Context, cols, rows uint32) (localTerminal, error) {
	if cols > 32767 || rows > 32767 {
		return nil, errors.New("ConPTY dimensions are invalid")
	}
	security := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	var inputRead, inputWrite, outputRead, outputWrite windows.Handle
	if err := windows.CreatePipe(&inputRead, &inputWrite, security, 0); err != nil {
		return nil, err
	}
	cleanup := func() {
		for _, handle := range []windows.Handle{inputRead, inputWrite, outputRead, outputWrite} {
			if handle != 0 {
				_ = windows.CloseHandle(handle)
			}
		}
	}
	if err := windows.CreatePipe(&outputRead, &outputWrite, security, 0); err != nil {
		cleanup()
		return nil, err
	}
	_ = windows.SetHandleInformation(inputWrite, windows.HANDLE_FLAG_INHERIT, 0)
	_ = windows.SetHandleInformation(outputRead, windows.HANDLE_FLAG_INHERIT, 0)
	var console windows.Handle
	if err := windows.CreatePseudoConsole(windows.Coord{X: int16(cols), Y: int16(rows)}, inputRead, outputWrite, 0, &console); err != nil {
		cleanup()
		return nil, err
	}
	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		windows.ClosePseudoConsole(console)
		cleanup()
		return nil, err
	}
	defer attributes.Delete()
	if err = attributes.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, unsafe.Pointer(&console), unsafe.Sizeof(console)); err != nil {
		windows.ClosePseudoConsole(console)
		cleanup()
		return nil, err
	}
	systemDirectory, err := windows.GetSystemDirectory()
	if err != nil {
		windows.ClosePseudoConsole(console)
		cleanup()
		return nil, err
	}
	executable := filepath.Join(systemDirectory, "WindowsPowerShell", "v1.0", "powershell.exe")
	application, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		windows.ClosePseudoConsole(console)
		cleanup()
		return nil, err
	}
	commandLine, err := windows.UTF16FromString(windows.ComposeCommandLine([]string{executable, "-NoLogo", "-NoProfile"}))
	if err != nil {
		windows.ClosePseudoConsole(console)
		cleanup()
		return nil, err
	}
	startup := &windows.StartupInfoEx{StartupInfo: windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{}))}, ProcThreadAttributeList: attributes.List()}
	process := new(windows.ProcessInformation)
	if err = windows.CreateProcess(application, &commandLine[0], nil, nil, true,
		windows.CREATE_UNICODE_ENVIRONMENT|windows.EXTENDED_STARTUPINFO_PRESENT, nil, nil, &startup.StartupInfo, process); err != nil {
		windows.ClosePseudoConsole(console)
		cleanup()
		return nil, err
	}
	_ = windows.CloseHandle(process.Thread)
	terminal := &windowsConPTY{input: os.NewFile(uintptr(inputWrite), "conpty-input"), output: os.NewFile(uintptr(outputRead), "conpty-output"),
		inputRead: inputRead, outputWrite: outputWrite, console: console, process: process.Process}
	inputWrite, outputRead = 0, 0
	go func() {
		<-ctx.Done()
		_ = terminal.Close()
	}()
	return terminal, nil
}

func (terminal *windowsConPTY) Read(value []byte) (int, error)  { return terminal.output.Read(value) }
func (terminal *windowsConPTY) Write(value []byte) (int, error) { return terminal.input.Write(value) }
func (terminal *windowsConPTY) Resize(cols, rows uint32) error {
	if cols > 32767 || rows > 32767 {
		return errors.New("ConPTY dimensions are invalid")
	}
	return windows.ResizePseudoConsole(terminal.console, windows.Coord{X: int16(cols), Y: int16(rows)})
}
func (terminal *windowsConPTY) Close() error {
	terminal.closeOnce.Do(func() {
		_ = windows.TerminateProcess(terminal.process, 0)
		windows.ClosePseudoConsole(terminal.console)
		_ = terminal.input.Close()
		_ = terminal.output.Close()
		_ = windows.CloseHandle(terminal.inputRead)
		_ = windows.CloseHandle(terminal.outputWrite)
		_ = windows.CloseHandle(terminal.process)
	})
	return nil
}
