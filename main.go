//go:build windows

package main

import (
	"os"
	"syscall"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
)

var Exe string

func init() {
	exe, err := os.Executable()
	if err != nil {
		panic(err)
	}
	Exe = exe
}

func main() {
	if isService, err := svc.IsWindowsService(); err != nil {
		panic(err)
	} else if isService {
		const WSM = "WSM"
		eventlog.InstallAsEventCreate(WSM, eventlog.Error|eventlog.Warning|eventlog.Info)
		elog, err := eventlog.Open(WSM)
		if err != nil {
			panic(err)
		}
		defer elog.Close()
		if err := svc.Run(WSM, &Handler{elog: elog}); err != nil {
			elog.Error(1, err.Error())
		}
		return
	}

	if !windows.GetCurrentProcessToken().IsElevated() {
		if err := elevate(); err != nil {
			panic(err)
		}
		return
	}

	app, err := NewApp()
	if err != nil {
		mygo.Dialog.Error("", NewError(err).Error())
		return
	}
	mygo.App.WhenReady(func() {
		app.win.Store(mygo.NewWindow(mygo.WindowOptions{
			Title:   "Windows Service Manager",
			Content: ui.View(app.view),
		}))
	})
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Label: "New", Click: func(item *mygo.MenuItem, win *mygo.Window) {
			win.Update(func() {
				app.form = ServiceConfig{StartType: AUTO_START}
				app.editing = true
				app.new = true
			})
		}},
		{Label: "Quit", Click: func(item *mygo.MenuItem, win *mygo.Window) {
			mygo.App.Quit()
		}},
	}))
	if err := mygo.App.Run(); err != nil {
		mygo.Dialog.Error("", NewError(err).Error())
	}
}

func elevate() error {
	verb, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := syscall.UTF16PtrFromString(Exe)
	if err != nil {
		return err
	}
	argsPtr, err := syscall.UTF16PtrFromString(Join(os.Args[1:]...))
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, argsPtr, nil, syscall.SW_NORMAL)
}
