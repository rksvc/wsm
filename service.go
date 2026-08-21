//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
)

var kernel32 = syscall.MustLoadDLL("kernel32.dll")

type Handler struct {
	name string
	elog *eventlog.Log
}

func (h *Handler) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	changes <- svc.Status{State: svc.StartPending}
	h.name = args[0]
	c, err := GetConfig(h.name)
	if err != nil {
		h.log(2, err)
		changes <- svc.Status{State: svc.StopPending}
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd, job, err := command(ctx, c, h)
	if err != nil {
		h.log(3, err)
		changes <- svc.Status{State: svc.StopPending}
		return
	}
	defer windows.CloseHandle(job)
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
loop:
	for {
		select {
		case <-ctx.Done():
			changes <- svc.Status{State: svc.StopPending}
			if err := cmd.Wait(); err != nil && !errors.Is(err, context.Canceled) {
				h.log(7, err)
			}

			var add int32
			r, _, err := kernel32.MustFindProc("SetConsoleCtrlHandler").Call(0, uintptr(unsafe.Pointer(&add)))
			if r == 0 {
				h.log(8, err)
			}

			if cmd.Stdin != nil {
				cmd.Stdin.(*os.File).Close()
			}
			if cmd.Stdout != nil {
				cmd.Stdout.(*os.File).Close()
			}
			if cmd.Stderr != nil && cmd.Stderr != cmd.Stdout {
				cmd.Stderr.(*os.File).Close()
			}
			break loop
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				cancel()
			}
		}
	}
	return
}

func (h *Handler) log(eid uint32, err error) {
	h.elog.Error(eid, fmt.Sprintf("[%s] %s", h.name, err))
}

func command(ctx context.Context, c *Config, h *Handler) (*exec.Cmd, windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, 0, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	_, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		return nil, 0, err
	}

	cmd := exec.CommandContext(ctx, c.Exe, c.Flags...)
	cmd.Dir = c.Dir
	cmd.Env = c.Env
	if c.Stdin != "" {
		f, err := os.Open(c.Stdin)
		if err != nil {
			return nil, 0, err
		}
		cmd.Stdin = f
	}
	if c.Stdout != "" {
		f, err := os.OpenFile(c.Stdout, os.O_APPEND|os.O_CREATE, 0666)
		if err != nil {
			return nil, 0, err
		}
		cmd.Stdout = f
	}
	if c.Stderr != "" {
		if c.Stderr == c.Stdout {
			cmd.Stderr = cmd.Stdout
		} else {
			f, err := os.OpenFile(c.Stderr, os.O_APPEND|os.O_CREATE, 0666)
			if err != nil {
				return nil, 0, err
			}
			cmd.Stderr = f
		}
	}
	cmd.Cancel = func() error {
		r, _, err := kernel32.MustFindProc("AttachConsole").Call(uintptr(cmd.Process.Pid))
		if r == 0 {
			switch err.(syscall.Errno) {
			case windows.ERROR_INVALID_HANDLE: // no console
				return cmd.Process.Kill()
			case windows.ERROR_GEN_FAILURE: // already exited
				return nil
			default:
				h.log(4, err)
				return err
			}
		}
		defer kernel32.MustFindProc("FreeConsole").Call()
		var add int32 = 1
		r, _, err = kernel32.MustFindProc("SetConsoleCtrlHandler").Call(0, uintptr(unsafe.Pointer(&add)))
		if r == 0 {
			h.log(5, err)
			return err
		}
		err = windows.GenerateConsoleCtrlEvent(syscall.CTRL_C_EVENT, 0)
		if err != nil {
			h.log(6, err)
		}
		return err
	}
	cmd.WaitDelay = 1500 * time.Millisecond

	if err := cmd.Start(); err != nil {
		return nil, 0, err
	}
	withHandleErr := cmd.Process.WithHandle(func(handle uintptr) {
		err = windows.AssignProcessToJobObject(job, windows.Handle(handle))
	})
	if withHandleErr != nil {
		return nil, 0, withHandleErr
	} else if err != nil {
		return nil, 0, err
	}
	return cmd, job, nil
}
