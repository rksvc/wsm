//go:build windows

package main

import (
	"context"
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

type Handler struct{}

func (h *Handler) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	changes <- svc.Status{State: svc.StartPending}
	c, err := GetConfig(args[0])
	if err != nil {
		changes <- svc.Status{State: svc.StopPending}
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd, job, err := command(ctx, c)
	if err != nil {
		changes <- svc.Status{State: svc.StopPending}
		return
	}
	defer windows.CloseHandle(job)
	go func() {
		cmd.Wait()
		cancel()
	}()
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
loop:
	for {
		select {
		case <-ctx.Done():
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
	changes <- svc.Status{State: svc.StopPending}
	return
}

func command(ctx context.Context, c *Config) (*exec.Cmd, windows.Handle, error) {
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
		f, err := os.OpenFile(c.Stdout, os.O_APPEND|os.O_CREATE, 0)
		if err != nil {
			return nil, 0, err
		}
		cmd.Stdout = f
	}
	if c.Stderr != "" {
		if c.Stderr == c.Stdout {
			cmd.Stderr = cmd.Stdout
		} else {
			f, err := os.OpenFile(c.Stderr, os.O_APPEND|os.O_CREATE, 0)
			if err != nil {
				return nil, 0, err
			}
			cmd.Stderr = f
		}
	}

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
